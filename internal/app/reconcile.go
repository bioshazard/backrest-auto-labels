package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/filters"

	"github.com/zettaio/backrest-sidecar/internal/config"
	"github.com/zettaio/backrest-sidecar/internal/docker"
	"github.com/zettaio/backrest-sidecar/internal/model"
)

// ReconcileOptions captures CLI flags for reconcile/daemon.
type ReconcileOptions struct {
	ConfigPath         string
	Apply              bool
	BackrestContainer  string
	DryRun             bool
	DockerSocket       string
	DockerRoot         string
	VolumePrefix       string
	DefaultRepo        string
	DefaultSchedule    string
	DefaultRetention   string
	PlanIDPrefix       string
	IncludeProjectName bool
	ExcludeBindMounts  bool
	Logger             *slog.Logger
	RestartTimeout     time.Duration
}

// Reconciler runs the main discovery/merge flow.
type Reconciler struct {
	opts     ReconcileOptions
	client   *docker.Client
	builder  *PlanBuilder
	log      *slog.Logger
	cfgPath  string
	dryRun   bool
	restarts struct {
		container string
		timeout   time.Duration
	}
}

// NewReconciler constructs a reconciler and Docker client.
func NewReconciler(opts ReconcileOptions) (*Reconciler, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	host := dockerHostFromSocket(opts.DockerSocket)
	client, err := docker.New(docker.Options{Host: host})
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	builder := NewPlanBuilder(PlanBuilderOptions{
		DockerRoot:         opts.DockerRoot,
		VolumePrefix:       opts.VolumePrefix,
		DefaultRepo:        opts.DefaultRepo,
		DefaultSchedule:    opts.DefaultSchedule,
		DefaultRetention:   opts.DefaultRetention,
		PlanIDPrefix:       opts.PlanIDPrefix,
		IncludeProjectName: opts.IncludeProjectName,
		ExcludeBindMounts:  opts.ExcludeBindMounts,
	})
	if opts.RestartTimeout == 0 {
		opts.RestartTimeout = 15 * time.Second
	}
	return &Reconciler{
		opts:    opts,
		client:  client,
		builder: builder,
		log:     opts.Logger,
		cfgPath: opts.ConfigPath,
		dryRun:  opts.DryRun,
		restarts: struct {
			container string
			timeout   time.Duration
		}{
			container: opts.BackrestContainer,
			timeout:   opts.RestartTimeout,
		},
	}, nil
}

// Close releases resources.
func (r *Reconciler) Close() {
	if r.client != nil {
		_ = r.client.Close()
	}
}

// Run executes a single reconcile pass.
func (r *Reconciler) Run(ctx context.Context) (*ReconcileResult, error) {
	cfg, _, err := config.Load(r.cfgPath)
	if err != nil {
		return nil, err
	}
	r.setDefaultRepoFromConfig(cfg)

	containers, err := r.client.ListBackrestEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	rendered := 0
	skipped := 0
	plans := make([]model.Plan, 0, len(containers))
	renderedPlans := make([]model.Plan, 0, len(containers))
	for _, ctr := range containers {
		plan, err := r.builder.Build(ctr)
		if err != nil {
			r.log.Warn("plan skipped", slog.String("container", ctr.Name), slog.String("id", ctr.ID[:12]), slog.String("error", err.Error()))
			skipped++
			continue
		}
		if !cfg.RepoExists(plan.Repo) {
			r.log.Warn("plan skipped - repo missing", slog.String("plan_id", plan.ID), slog.String("repo", plan.Repo))
			skipped++
			continue
		}
		plans = append(plans, *plan)
		renderedPlans = append(renderedPlans, *plan)
		rendered++
	}

	changed, changedIDs := cfg.UpsertPlans(plans)

	changedSet := make(map[string]struct{}, len(changedIDs))
	for _, id := range changedIDs {
		changedSet[id] = struct{}{}
	}
	for _, plan := range renderedPlans {
		args := []any{
			slog.String("plan_id", plan.ID),
			slog.String("repo", plan.Repo),
			slog.Any("paths", plan.Paths),
			slog.Bool("dry_run", r.dryRun),
		}
		if _, ok := changedSet[plan.ID]; ok {
			r.log.Info("plan.rendered", args...)
		} else {
			r.log.Debug("plan.rendered", args...)
		}
	}

	if !changed {
		r.log.Debug("reconcile.complete", slog.Int("rendered", rendered), slog.Int("skipped", skipped), slog.Bool("changed", false))
		return &ReconcileResult{PlansSeen: rendered, PlansChanged: 0, Changed: false}, nil
	}

	cfg.Normalize()
	if r.dryRun {
		r.log.Info("dry-run.complete", slog.Int("plans_seen", rendered), slog.Int("plans_changed", len(changedIDs)), slog.String("config", r.cfgPath))
		return &ReconcileResult{PlansSeen: rendered, PlansChanged: len(changedIDs), Changed: true, DryRun: true}, nil
	}

	if _, err := config.Write(r.cfgPath, cfg); err != nil {
		return nil, err
	}
	r.log.Info("config.write", slog.String("path", r.cfgPath), slog.Int("plans_total", len(cfg.Plans)), slog.Any("plans_changed", changedIDs))

	if r.opts.Apply && r.restarts.container != "" {
		if err := r.client.RestartContainer(ctx, r.restarts.container, r.restarts.timeout); err != nil {
			return nil, fmt.Errorf("restart backrest container: %w", err)
		}
		r.log.Info("backrest.restart", slog.String("container", r.restarts.container))
	}

	r.log.Info("reconcile.complete", slog.Int("rendered", rendered), slog.Int("skipped", skipped), slog.Bool("changed", true))
	return &ReconcileResult{PlansSeen: rendered, PlansChanged: len(changedIDs), Changed: true}, nil
}

func (r *Reconciler) setDefaultRepoFromConfig(cfg *model.Config) {
	if cfg == nil || len(cfg.Repos) == 0 {
		return
	}
	current := strings.TrimSpace(r.builder.opts.DefaultRepo)
	if current != "" && cfg.RepoExists(current) {
		return
	}
	r.builder.opts.DefaultRepo = cfg.Repos[0].ID
}

// ReconcileResult summarises the reconcile run.
type ReconcileResult struct {
	PlansSeen    int
	PlansChanged int
	Changed      bool
	DryRun       bool
}

// DaemonOptions extends reconcile options with scheduling knobs.
type DaemonOptions struct {
	ReconcileOptions
	Interval   time.Duration
	WithEvents bool
}

// RunDaemon loops reconcile on a timer (and optionally on docker events).
func RunDaemon(ctx context.Context, opts DaemonOptions) error {
	reconciler, err := NewReconciler(opts.ReconcileOptions)
	if err != nil {
		return err
	}
	defer reconciler.Close()

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	trigger := make(chan struct{}, 1)
	trigger <- struct{}{} // run immediately

	var (
		eventCtx context.Context
		cancel   context.CancelFunc
	)

	if opts.WithEvents {
		eventCtx, cancel = context.WithCancel(ctx)
		filterArgs := filters.NewArgs()
		filterArgs.Add("type", "container")
		msgCh, errCh := reconciler.client.Events(eventCtx, filterArgs)
		go func() {
			for {
				select {
				case <-eventCtx.Done():
					return
				case _, ok := <-msgCh:
					if !ok {
						return
					}
					select {
					case trigger <- struct{}{}:
					default:
					}
				case err := <-errCh:
					if err != nil && !errors.Is(err, context.Canceled) {
						reconciler.log.Warn("docker events", slog.String("error", err.Error()))
					}
					return
				}
			}
		}()
		defer cancel()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			select {
			case trigger <- struct{}{}:
			default:
			}
		case <-trigger:
			if _, err := reconciler.Run(ctx); err != nil {
				reconciler.log.Error("reconcile failed", slog.String("error", err.Error()))
			}
		}
	}
}

func dockerHostFromSocket(sock string) string {
	if sock == "" {
		return ""
	}
	if strings.HasPrefix(sock, "unix://") || strings.HasPrefix(sock, "tcp://") {
		return sock
	}
	return "unix://" + filepath.Clean(sock)
}
