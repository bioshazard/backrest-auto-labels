package model

import (
	"encoding/json"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Plan represents a Backrest plan.
type Plan struct {
	ID           string        `json:"id"`
	Repo         string        `json:"repo"`
	Paths        []string      `json:"paths"`
	PathsExclude []string      `json:"pathsExclude,omitempty"`
	Schedule     PlanSchedule  `json:"schedule"`
	Retention    PlanRetention `json:"retention"`
	Hooks        []PlanHook    `json:"hooks,omitempty"`
}

type PlanSchedule struct {
	Cron  string `json:"cron"`
	Clock string `json:"clock"`
}

type PlanRetention struct {
	PolicyTimeBucketed *RetentionBuckets `json:"policyTimeBucketed,omitempty"`
	spec               string
}

type RetentionBuckets struct {
	Hourly  int `json:"hourly,omitempty"`
	Daily   int `json:"daily,omitempty"`
	Weekly  int `json:"weekly,omitempty"`
	Monthly int `json:"monthly,omitempty"`
	Yearly  int `json:"yearly,omitempty"`
}

type PlanHook struct {
	Conditions    []string    `json:"conditions"`
	ActionCommand HookCommand `json:"actionCommand"`
}

type HookCommand struct {
	Command string `json:"command"`
}

// Repo represents a Backrest repo entry.
type Repo struct {
	ID   string   `json:"id"`
	Type string   `json:"type,omitempty"`
	URL  string   `json:"url,omitempty"`
	URI  string   `json:"uri,omitempty"`
	Env  []string `json:"env,omitempty"`
}

// Config is the Backrest config file model.
type Config struct {
	Repos    []Repo `json:"repos"`
	Plans    []Plan `json:"plans"`
	extras   map[string]json.RawMessage
	rawRepos json.RawMessage
}

// EnsureNonNil ensures slices/maps are initialized.
func (c *Config) EnsureNonNil() {
	if c.Repos == nil {
		c.Repos = []Repo{}
	}
	if c.Plans == nil {
		c.Plans = []Plan{}
	}
	if c.extras == nil {
		c.extras = make(map[string]json.RawMessage)
	}
}

// Extras returns raw top-level fields outside repos/plans.
func (c *Config) Extras() map[string]json.RawMessage {
	c.EnsureNonNil()
	return c.extras
}

// SetExtras replaces the raw extras map.
func (c *Config) SetExtras(raw map[string]json.RawMessage) {
	c.extras = raw
}

// RawRepos returns the raw repos JSON if it was present when loading the config.
func (c *Config) RawRepos() json.RawMessage {
	if len(c.rawRepos) == 0 {
		return nil
	}
	return append([]byte(nil), c.rawRepos...)
}

// SetRawRepos stores the raw repos JSON so it can be re-emitted without losing extra fields.
func (c *Config) SetRawRepos(raw json.RawMessage) {
	if len(raw) == 0 {
		c.rawRepos = nil
		return
	}
	c.rawRepos = append([]byte(nil), raw...)
}

// ClearRawRepos discards the stored raw repos payload so the next write re-marshals the struct.
func (c *Config) ClearRawRepos() {
	c.rawRepos = nil
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
	slices.Sort(p.Paths)
	p.Paths = uniqueStrings(p.Paths)

	slices.Sort(p.PathsExclude)
	p.PathsExclude = uniqueStrings(p.PathsExclude)

	hookRank := func(conditions []string) int {
		if len(conditions) == 0 {
			return 99
		}
		switch conditions[0] {
		case "CONDITION_SNAPSHOT_START":
			return 0
		case "CONDITION_SNAPSHOT_END":
			return 1
		default:
			return 2
		}
	}
	sort.Slice(p.Hooks, func(i, j int) bool {
		ri := hookRank(p.Hooks[i].Conditions)
		rj := hookRank(p.Hooks[j].Conditions)
		if ri == rj {
			return p.Hooks[i].ActionCommand.Command < p.Hooks[j].ActionCommand.Command
		}
		return ri < rj
	})
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
	LabelHookSnapshotStart = "backrest.snapshot-start"
	LabelHookSnapshotEnd   = "backrest.snapshot-end"
	LabelHooksTemplate     = "backrest.hooks.template"
	LabelRetentionKeep     = "backrest.keep"
	LabelQuiesce           = "backrest.quiesce"
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

// RetentionFromSpec converts "daily=7" style specs into buckets.
func (p *PlanRetention) RetentionFromSpec(spec string) {
	p.spec = spec
	if spec == "" {
		p.PolicyTimeBucketed = nil
		return
	}
	buckets := &RetentionBuckets{}
	parts := strings.Split(spec, ",")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.TrimSpace(kv[1])
		n, err := strconv.Atoi(val)
		if err != nil {
			continue
		}
		switch key {
		case "hourly":
			buckets.Hourly = n
		case "daily":
			buckets.Daily = n
		case "weekly":
			buckets.Weekly = n
		case "monthly":
			buckets.Monthly = n
		case "yearly":
			buckets.Yearly = n
		}
	}
	if *buckets == (RetentionBuckets{}) {
		p.PolicyTimeBucketed = nil
		return
	}
	p.PolicyTimeBucketed = buckets
}

func (p PlanRetention) Spec() string {
	return p.spec
}
