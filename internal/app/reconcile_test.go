package app

import (
	"testing"

	"github.com/zettaio/backrest-sidecar/internal/model"
)

func TestSetDefaultRepoFromConfigPrefersFirstPlanWhenUnset(t *testing.T) {
	cfg := &model.Config{
		Plans: []model.Plan{
			{Repo: "plan-alpha"},
			{Repo: "plan-beta"},
		},
		Repos: []model.Repo{{ID: "repo-entry"}},
	}
	r := testReconcilerWithDefault("default", false)
	r.setDefaultRepoFromConfig(cfg)
	if got, want := r.builder.opts.DefaultRepo, "plan-alpha"; got != want {
		t.Fatalf("expected fallback to first plan repo %q, got %q", want, got)
	}
}

func TestSetDefaultRepoFromConfigFallsBackToRepoListWhenNoPlans(t *testing.T) {
	cfg := &model.Config{
		Repos: []model.Repo{
			{ID: "repo-one"},
			{ID: "repo-two"},
		},
	}
	r := testReconcilerWithDefault("", false)
	r.setDefaultRepoFromConfig(cfg)
	if got, want := r.builder.opts.DefaultRepo, "repo-one"; got != want {
		t.Fatalf("expected fallback to first repo %q, got %q", want, got)
	}
}

func TestSetDefaultRepoFromConfigRespectsExplicitDefault(t *testing.T) {
	cfg := &model.Config{
		Repos: []model.Repo{{ID: "custom"}},
	}
	r := testReconcilerWithDefault("custom", true)
	r.setDefaultRepoFromConfig(cfg)
	if got, want := r.builder.opts.DefaultRepo, "custom"; got != want {
		t.Fatalf("expected explicit default repo %q to be preserved, got %q", want, got)
	}
}

func TestSetDefaultRepoFromConfigOverridesMissingExplicitDefault(t *testing.T) {
	cfg := &model.Config{
		Plans: []model.Plan{{Repo: "plan-alpha"}},
		Repos: []model.Repo{{ID: "repo-entry"}},
	}
	r := testReconcilerWithDefault("does-not-exist", true)
	r.setDefaultRepoFromConfig(cfg)
	if got, want := r.builder.opts.DefaultRepo, "plan-alpha"; got != want {
		t.Fatalf("expected fallback to plan repo %q when explicit default missing, got %q", want, got)
	}
}

func testReconcilerWithDefault(defaultRepo string, provided bool) *Reconciler {
	return &Reconciler{
		builder:             NewPlanBuilder(PlanBuilderOptions{DefaultRepo: defaultRepo}),
		defaultRepoProvided: provided,
	}
}
