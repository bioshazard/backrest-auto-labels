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
	if len(pl.Hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(pl.Hooks))
	}
	expect := []struct {
		cond string
		cmd  string
	}{
		{"CONDITION_SNAPSHOT_START", "docker stop demo-echo-lite-1"},
		{"CONDITION_SNAPSHOT_END", "docker start demo-echo-lite-1"},
	}
	for i, hook := range pl.Hooks {
		if len(hook.Conditions) != 1 || hook.Conditions[0] != expect[i].cond {
			t.Fatalf("hook %d condition mismatch: got %v want %s", i, hook.Conditions, expect[i].cond)
		}
		if hook.ActionCommand.Command != expect[i].cmd {
			t.Fatalf("hook %d command mismatch: got %s want %s", i, hook.ActionCommand.Command, expect[i].cmd)
		}
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
	if len(pl.Hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(pl.Hooks))
	}
	hook := pl.Hooks[0]
	if len(hook.Conditions) != 1 || hook.Conditions[0] != "CONDITION_SNAPSHOT_START" {
		t.Fatalf("unexpected conditions: %v", hook.Conditions)
	}
	if hook.ActionCommand.Command != "echo noop" {
		t.Fatalf("unexpected command: %s", hook.ActionCommand.Command)
	}
}
