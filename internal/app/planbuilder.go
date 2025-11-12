package app

import (
	"fmt"
	"path/filepath"
	"strings"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/mount"

	"github.com/zettaio/backrest-sidecar/internal/docker"
	"github.com/zettaio/backrest-sidecar/internal/model"
)

// PlanBuilderOptions configures plan synthesis.
type PlanBuilderOptions struct {
	DockerRoot         string
	VolumePrefix       string
	DefaultRepo        string
	DefaultSchedule    string
	DefaultRetention   string
	PlanIDPrefix       string
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

	retention := strings.TrimSpace(container.Labels[model.LabelRetentionKeep])
	if retention == "" {
		retention = strings.TrimSpace(b.opts.DefaultRetention)
	}

	plan := &model.Plan{
		ID:        id,
		Repo:      repo,
		Schedule:  schedule,
		Sources:   sources,
		Exclude:   exclude,
		Hooks:     hooks,
		Retention: model.RetSpec{Spec: retention},
	}
	plan.Normalize()
	return plan, nil
}

func (b *PlanBuilder) planID(container docker.Container) string {
	base := b.basePlanID(container)
	if base == "" {
		return ""
	}
	if b.opts.PlanIDPrefix == "" {
		return base
	}
	return sanitizeID(b.opts.PlanIDPrefix + base)
}

func (b *PlanBuilder) basePlanID(container docker.Container) string {
	project := strings.TrimSpace(container.Project)
	service := strings.TrimSpace(container.Service)
	var raw string
	switch {
	case project != "" && service != "" && b.opts.IncludeProjectName:
		raw = project + "_" + service
	case service != "":
		raw = service
	default:
		if container.Name != "" {
			raw = container.Name
			break
		}
		id := container.ID
		if len(id) > 12 {
			id = id[:12]
		}
		raw = id
	}
	return sanitizeID(raw)
}

func (b *PlanBuilder) sources(container docker.Container) []string {
	if labels := model.ParseCSV(container.Labels[model.LabelPathsInclude]); len(labels) > 0 {
		return b.rewriteLabeledPaths(labels, container.Mounts)
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
				hostPath := filepath.Join(b.opts.DockerRoot, "volumes", m.Name, "_data")
				paths = append(paths, b.rewriteVolumePath(hostPath))
			}
		}
	}
	return unique(paths)
}

func (b *PlanBuilder) rewriteVolumePath(path string) string {
	if b.opts.VolumePrefix == "" {
		return path
	}
	base := filepath.Join(b.opts.DockerRoot, "volumes")
	rel, err := filepath.Rel(base, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return filepath.Join(b.opts.VolumePrefix, rel)
}

func (b *PlanBuilder) rewriteLabeledPaths(paths []string, mounts []dockertypes.MountPoint) []string {
	if len(paths) == 0 {
		return paths
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rewritten := b.hostPathForLabel(p, mounts)
		if rewritten == "" {
			if p != "" {
				out = append(out, p)
			}
			continue
		}
		out = append(out, rewritten)
	}
	return unique(out)
}

func (b *PlanBuilder) hostPathForLabel(path string, mounts []dockertypes.MountPoint) string {
	cleanLabel := filepath.Clean(path)
	for _, m := range mounts {
		target := filepath.Clean(m.Destination)
		if target == "." || target == "" {
			continue
		}
		rel, ok := relWithin(cleanLabel, target)
		if !ok {
			continue
		}
		switch m.Type {
		case mount.TypeVolume:
			if m.Name == "" {
				continue
			}
			hostPath := filepath.Join(b.opts.DockerRoot, "volumes", m.Name, "_data")
			if rel != "" {
				hostPath = filepath.Join(hostPath, rel)
			}
			return b.rewriteVolumePath(hostPath)
		case mount.TypeBind:
			if m.Source == "" {
				continue
			}
			hostPath := m.Source
			if rel != "" {
				hostPath = filepath.Join(hostPath, rel)
			}
			return hostPath
		}
	}
	return ""
}

func relWithin(path, base string) (string, bool) {
	if path == "" || base == "" {
		return "", false
	}
	path = filepath.Clean(path)
	base = filepath.Clean(base)
	if path == base {
		return "", true
	}
	prefix := base
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix), true
	}
	return "", false
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
