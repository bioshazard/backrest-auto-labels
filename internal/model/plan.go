package model

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
)

// Plan represents a Backrest plan.
type Plan struct {
	ID        string   `json:"id"`
	Repo      string   `json:"repo"`
	Schedule  string   `json:"schedule"`
	Sources   []string `json:"sources"`
	Exclude   []string `json:"exclude,omitempty"`
	Hooks     Hooks    `json:"hooks,omitempty"`
	Retention RetSpec  `json:"retention,omitempty"`
}

// Hooks describe pre/post commands.
type Hooks struct {
	Pre  []string `json:"pre,omitempty"`
	Post []string `json:"post,omitempty"`
}

// RetSpec stores the retention spec string.
type RetSpec struct {
	Spec string `json:"spec,omitempty"`
}

// Repo represents a Backrest repo entry.
type Repo struct {
	ID   string            `json:"id"`
	Type string            `json:"type"`
	URL  string            `json:"url"`
	Env  map[string]string `json:"env,omitempty"`
}

// Config is the Backrest config file model.
type Config struct {
	Repos []Repo `json:"repos"`
	Plans []Plan `json:"plans"`
}

// EnsureNonNil ensures slices/maps are initialized.
func (c *Config) EnsureNonNil() {
	if c.Repos == nil {
		c.Repos = []Repo{}
	}
	if c.Plans == nil {
		c.Plans = []Plan{}
	}
}

// RepoExists returns true if repo ID exists.
func (c *Config) RepoExists(id string) bool {
	for _, r := range c.Repos {
		if r.ID == id {
			return true
		}
	}
	return false
}

// UpsertPlans merges the provided plans by ID into the config and returns the IDs that changed.
func (c *Config) UpsertPlans(plans []Plan) (bool, []string) {
	if len(plans) == 0 {
		return false, nil
	}
	c.EnsureNonNil()
	for idx := range plans {
		plans[idx].Normalize()
	}

	changedIDs := make([]string, 0, len(plans))
	for _, plan := range plans {
		found := false
		for i := range c.Plans {
			if c.Plans[i].ID == plan.ID {
				found = true
				if !plansEqual(c.Plans[i], plan) {
					c.Plans[i] = plan
					changedIDs = append(changedIDs, plan.ID)
				}
				break
			}
		}
		if !found {
			c.Plans = append(c.Plans, plan)
			changedIDs = append(changedIDs, plan.ID)
		}
	}

	sort.Slice(c.Plans, func(i, j int) bool {
		return c.Plans[i].ID < c.Plans[j].ID
	})
	return len(changedIDs) > 0, changedIDs
}

// Normalize ensures config slices are sorted deterministically.
func (c *Config) Normalize() {
	c.EnsureNonNil()
	for i := range c.Plans {
		c.Plans[i].Normalize()
	}
	sort.Slice(c.Plans, func(i, j int) bool {
		return c.Plans[i].ID < c.Plans[j].ID
	})
}

// Normalize sorts slices for deterministic output.
func (p *Plan) Normalize() {
	slices.Sort(p.Sources)
	p.Sources = uniqueStrings(p.Sources)

	slices.Sort(p.Exclude)
	p.Exclude = uniqueStrings(p.Exclude)

	slices.Sort(p.Hooks.Pre)
	p.Hooks.Pre = uniqueStrings(p.Hooks.Pre)

	slices.Sort(p.Hooks.Post)
	p.Hooks.Post = uniqueStrings(p.Hooks.Post)
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := in[:0]
	var last string
	for i, v := range in {
		if i == 0 || v != last {
			out = append(out, v)
			last = v
		}
	}
	return out
}

func plansEqual(a, b Plan) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

// Label helpers -------------------------------------------------------------

const (
	LabelEnable            = "backrest.enable"
	LabelRepo              = "backrest.repo"
	LabelSchedule          = "backrest.schedule"
	LabelPathsInclude      = "backrest.paths.include"
	LabelPathsExclude      = "backrest.paths.exclude"
	LabelHooksPre          = "backrest.pre"
	LabelHooksPost         = "backrest.post"
	LabelRetentionKeep     = "backrest.keep"
	LabelRCBVolumes        = "restic-compose-backup.volumes"
	LabelRCBVolumesInclude = "restic-compose-backup.volumes.include"
	LabelRCBVolumesExclude = "restic-compose-backup.volumes.exclude"
	LabelRCBQuiesce        = "restic-compose-backup.quiesce"
	LabelSidecarQuiesce    = "restic-compose-backup.quiesce"
	LabelComposeProject    = "com.docker.compose.project"
	LabelComposeService    = "com.docker.compose.service"
)

// ParseCSV splits comma separated values, trimming whitespace.
func ParseCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
