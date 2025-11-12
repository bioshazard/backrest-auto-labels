package app

import (
	"testing"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/mount"

	"github.com/zettaio/backrest-sidecar/internal/docker"
	"github.com/zettaio/backrest-sidecar/internal/model"
)

func TestPlanBuilderHookTemplateStopStart(t *testing.T) {
	b := NewPlanBuilder(PlanBuilderOptions{
		DockerRoot:       "/var/lib/docker",
		DefaultRepo:      "sample-repo",
		DefaultSchedule:  "0 2 * * *",
		DefaultRetention: "daily=7,weekly=4",
		PlanIDPrefix:     "backrest_sidecar_",
	})
	ctr := docker.Container{
		Name: "demo-echo-lite-1",
		Labels: map[string]string{
			model.LabelRepo:          "sample-repo",
			model.LabelSchedule:      "0 2 * * *",
			model.LabelHooksTemplate: "simple-stop-start",
			model.LabelPathsInclude:  "/srv/data",
		},
		Mounts: []dockertypes.MountPoint{{
			Type:        mount.TypeBind,
			Source:      "/host/data",
			Destination: "/srv/data",
		}},
	}
	pl, err := b.Build(ctr)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if got, want := pl.Hooks.Pre, []string{"docker stop demo-echo-lite-1"}; !slicesEqual(got, want) {
		t.Fatalf("pre hooks mismatch: got %v want %v", got, want)
	}
	if got, want := pl.Hooks.Post, []string{"docker start demo-echo-lite-1"}; !slicesEqual(got, want) {
		t.Fatalf("post hooks mismatch: got %v want %v", got, want)
	}
}

func TestPlanBuilderHookTemplateDoesNotOverrideManualHooks(t *testing.T) {
	b := NewPlanBuilder(PlanBuilderOptions{
		DockerRoot:       "/var/lib/docker",
		DefaultRepo:      "sample-repo",
		DefaultSchedule:  "0 2 * * *",
		DefaultRetention: "daily=7,weekly=4",
	})
	ctr := docker.Container{
		Name: "demo-echo",
		Labels: map[string]string{
			model.LabelRepo:          "sample-repo",
			model.LabelSchedule:      "0 2 * * *",
			model.LabelHooksTemplate: "simple-stop-start",
			model.LabelHooksPre:      "echo noop",
			model.LabelPathsInclude:  "/data",
		},
		Mounts: []dockertypes.MountPoint{{
			Type:        mount.TypeVolume,
			Name:        "demo-data",
			Destination: "/data",
		}},
	}
	pl, err := b.Build(ctr)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if got, want := pl.Hooks.Pre, []string{"echo noop"}; !slicesEqual(got, want) {
		t.Fatalf("pre hooks mismatch: got %v want %v", got, want)
	}
	if len(pl.Hooks.Post) != 0 {
		t.Fatalf("expected no post hooks, got %v", pl.Hooks.Post)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
