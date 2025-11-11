package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zettaio/backrest-sidecar/internal/docker"
	"github.com/zettaio/backrest-sidecar/internal/model"
	"github.com/zettaio/backrest-sidecar/internal/util/exec"
)

// BackupOptions configures backup-once behavior.
type BackupOptions struct {
	DockerSocket       string
	DockerRoot         string
	IncludeProjectName bool
	ExcludeBindMounts  bool
	Logger             *slog.Logger

	RCBImage     string
	RCBCommand   []string
	RCBEnvFile   string
	RCBExtraArgs []string

	QuiesceLabel   string
	QuiesceTimeout time.Duration

	ResticGroupBy    string
	ResticPathPrefix string
}

// RunBackup runs the one-shot backup pipeline.
func RunBackup(ctx context.Context, opts BackupOptions) error {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.RCBImage == "" {
		return errors.New("rcb image required")
	}
	if len(opts.RCBCommand) == 0 {
		opts.RCBCommand = []string{"rcb", "backup"}
	}
	if opts.QuiesceTimeout == 0 {
		opts.QuiesceTimeout = 60 * time.Second
	}
	if opts.ResticGroupBy == "" {
		opts.ResticGroupBy = "paths"
	}
	if opts.ResticPathPrefix == "" {
		opts.ResticPathPrefix = "/volumes"
	}

	client, err := docker.New(docker.Options{Host: dockerHostFromSocket(opts.DockerSocket)})
	if err != nil {
		return err
	}
	defer client.Close()

	stopped, stopErr := quiesceContainers(ctx, client, opts)
	defer func() {
		for _, ctr := range stopped {
			if err := client.StartContainer(ctx, ctr.ID); err != nil {
				opts.Logger.Error("quiesce.start_failed", slog.String("container", ctr.Name), slog.String("error", err.Error()))
			} else {
				opts.Logger.Info("quiesce.started", slog.String("container", ctr.Name))
			}
		}
	}()
	if stopErr != nil {
		return stopErr
	}

	if err := runRcbContainer(ctx, opts, opts.RCBCommand, opts.RCBExtraArgs); err != nil {
		return err
	}

	if err := runRetention(ctx, client, opts); err != nil {
		return err
	}

	opts.Logger.Info("backup-once.complete")
	return nil
}

func quiesceContainers(ctx context.Context, client *docker.Client, opts BackupOptions) ([]docker.Container, error) {
	if opts.QuiesceLabel == "" {
		return nil, nil
	}
	containers, err := client.ListByLabel(ctx, opts.QuiesceLabel)
	if err != nil {
		return nil, err
	}
	stopped := make([]docker.Container, 0, len(containers))
	for _, ctr := range containers {
		if ctr.State != "running" {
			continue
		}
		opts.Logger.Info("quiesce.stop", slog.String("container", ctr.Name))
		if err := client.StopContainer(ctx, ctr.ID, opts.QuiesceTimeout); err != nil {
			return stopped, fmt.Errorf("stop container %s: %w", ctr.Name, err)
		}
		stopped = append(stopped, ctr)
	}
	return stopped, nil
}

func runRetention(ctx context.Context, client *docker.Client, opts BackupOptions) error {
	containers, err := client.ListBackrestEnabled(ctx)
	if err != nil {
		return err
	}
	for _, ctr := range containers {
		spec := strings.TrimSpace(ctr.Labels[model.LabelRetentionKeep])
		if spec == "" {
			continue
		}
		path := resticPath(opts, ctr)
		if path == "" {
			continue
		}
		flags := retentionFlags(spec)
		if len(flags) == 0 {
			continue
		}
		cmd := []string{"restic", "forget", "--group-by", opts.ResticGroupBy, "--path", path}
		cmd = append(cmd, flags...)
		cmd = append(cmd, "--prune")
		opts.Logger.Info("retention.run", slog.String("container", ctr.Name), slog.String("path", path))
		if err := runRcbContainer(ctx, opts, cmd, nil); err != nil {
			return fmt.Errorf("restic forget %s: %w", ctr.Name, err)
		}
	}
	return nil
}

func resticPath(opts BackupOptions, ctr docker.Container) string {
	service := ctr.Service
	if service == "" {
		service = ctr.Name
	}
	name := serviceName(ctr.Project, service, ctr.Name, opts.IncludeProjectName)
	if name == "" {
		return ""
	}
	return filepath.Join(opts.ResticPathPrefix, filepath.FromSlash(name))
}

func retentionFlags(spec string) []string {
	pairs := strings.Split(spec, ",")
	flags := make([]string, 0, len(pairs)*2)
	for _, pair := range pairs {
		p := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(p) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(p[0]))
		val := strings.TrimSpace(p[1])
		if val == "" {
			continue
		}
		switch key {
		case "last":
			flags = append(flags, "--keep-last", val)
		case "hourly":
			flags = append(flags, "--keep-hourly", val)
		case "daily":
			flags = append(flags, "--keep-daily", val)
		case "weekly":
			flags = append(flags, "--keep-weekly", val)
		case "monthly":
			flags = append(flags, "--keep-monthly", val)
		case "yearly":
			flags = append(flags, "--keep-yearly", val)
		case "within":
			flags = append(flags, "--keep-within", val)
		case "within-d":
			flags = append(flags, "--keep-within-d", val)
		case "within-w":
			flags = append(flags, "--keep-within-w", val)
		case "within-m":
			flags = append(flags, "--keep-within-m", val)
		case "within-y":
			flags = append(flags, "--keep-within-y", val)
		default:
			continue
		}
	}
	return flags
}

func runRcbContainer(ctx context.Context, opts BackupOptions, command []string, extra []string) error {
	socketPath, err := dockerSocketPath(opts.DockerSocket)
	if err != nil {
		return err
	}

	args := []string{"run", "--rm"}
	if socketPath != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/tmp/docker.sock:ro", socketPath))
	}
	if opts.DockerRoot != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/var/lib/docker:ro", filepath.Clean(opts.DockerRoot)))
	}
	if opts.RCBEnvFile != "" {
		args = append(args, "--env-file", opts.RCBEnvFile)
	}
	envVars := []string{
		fmt.Sprintf("EXCLUDE_BIND_MOUNTS=%d", boolToInt(opts.ExcludeBindMounts)),
		fmt.Sprintf("INCLUDE_PROJECT_NAME=%d", boolToInt(opts.IncludeProjectName)),
	}
	for _, env := range envVars {
		args = append(args, "-e", env)
	}

	args = append(args, opts.RCBImage)
	args = append(args, command...)
	if len(extra) > 0 {
		args = append(args, extra...)
	}

	return executil.Run(ctx, "docker", args, executil.RunOptions{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
}

func dockerSocketPath(raw string) (string, error) {
	if raw == "" {
		return "/var/run/docker.sock", nil
	}
	if strings.HasPrefix(raw, "unix://") {
		raw = strings.TrimPrefix(raw, "unix://")
	}
	if strings.HasPrefix(raw, "tcp://") {
		return "", fmt.Errorf("docker run requires local socket, got tcp host %s", raw)
	}
	return filepath.Clean(raw), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
