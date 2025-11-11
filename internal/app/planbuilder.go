package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/mount"

	"github.com/zettaio/backrest-sidecar/internal/docker"
	"github.com/zettaio/backrest-sidecar/internal/model"
)

// PlanBuilderOptions configures plan synthesis.
type PlanBuilderOptions struct {
	DockerRoot         string
	DefaultRepo        string
	DefaultSchedule    string
	IncludeProjectName bool
	ExcludeBindMounts  bool
}

// PlanBuilder converts Docker containers into Backrest plans.
type PlanBuilder struct {
	opts PlanBuilderOptions
}

// NewPlanBuilder returns a builder with the provided options.
func NewPlanBuilder(opts PlanBuilderOptions) *PlanBuilder {
	return &PlanBuilder{opts: opts}
}

// Build constructs a plan or returns error if the container cannot be represented.
func (b *PlanBuilder) Build(container docker.Container) (*model.Plan, error) {
	repo := model.GetLabel(container.Labels, model.LabelRepo, b.opts.DefaultRepo)
	if repo == "" {
		return nil, fmt.Errorf("container %s missing repo label and default repo", container.Name)
	}
	schedule := model.GetLabel(container.Labels, model.LabelSchedule, b.opts.DefaultSchedule)
	if schedule == "" {
		return nil, fmt.Errorf("container %s missing schedule label and default", container.Name)
	}

	id := b.planID(container)
	if id == "" {
		return nil, fmt.Errorf("unable to derive plan id for container %s", container.Name)
	}

	sources := b.sources(container)
	if len(sources) == 0 {
		return nil, fmt.Errorf("container %s has no derived sources; add backrest.paths.include", container.Name)
	}

	exclude := model.ParseCSV(container.Labels[model.LabelPathsExclude])
	hooks := model.Hooks{
		Pre:  model.ParseCSV(container.Labels[model.LabelHooksPre]),
		Post: model.ParseCSV(container.Labels[model.LabelHooksPost]),
	}

	plan := &model.Plan{
		ID:        id,
		Repo:      repo,
		Schedule:  schedule,
		Sources:   sources,
		Exclude:   exclude,
		Hooks:     hooks,
		Retention: model.RetSpec{Spec: strings.TrimSpace(container.Labels[model.LabelRetentionKeep])},
	}
	plan.Normalize()
	return plan, nil
}

func (b *PlanBuilder) planID(container docker.Container) string {
	project := strings.TrimSpace(container.Project)
	service := strings.TrimSpace(container.Service)
	switch {
	case project != "" && service != "" && b.opts.IncludeProjectName:
		return sanitizeID(project + "_" + service)
	case service != "":
		return sanitizeID(service)
	default:
		if container.Name != "" {
			return sanitizeID(container.Name)
		}
		id := container.ID
		if len(id) > 12 {
			id = id[:12]
		}
		return sanitizeID(id)
	}
}

func (b *PlanBuilder) sources(container docker.Container) []string {
	if labels := model.ParseCSV(container.Labels[model.LabelPathsInclude]); len(labels) > 0 {
		return labels
	}
	if len(container.Mounts) == 0 {
		return nil
	}
	paths := make([]string, 0, len(container.Mounts))
	for _, m := range container.Mounts {
		switch m.Type {
		case mount.TypeBind:
			if b.opts.ExcludeBindMounts {
				continue
			}
			if m.Source != "" {
				paths = append(paths, m.Source)
			}
		case mount.TypeVolume:
			if m.Name != "" {
				paths = append(paths, filepath.Join(b.opts.DockerRoot, "volumes", m.Name, "_data"))
			}
		}
	}
	return unique(paths)
}

func unique(items []string) []string {
	if len(items) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
