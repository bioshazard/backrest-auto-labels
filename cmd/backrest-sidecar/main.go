package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/zettaio/backrest-sidecar/internal/app"
)

var version = "dev"

var exitCode = 0

func main() {
	rootCmd := newRootCmd()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

type commonFlags struct {
	configPath         string
	apply              bool
	backrestContainer  string
	dryRun             bool
	dockerSocket       string
	dockerRoot         string
	volumePrefix       string
	defaultRepo        string
	defaultSchedule    string
	includeProjectName bool
	excludeBindMounts  bool
	restartTimeout     time.Duration
	logFormat          string
	logLevel           string
}

func newRootCmd() *cobra.Command {
	flags := commonFlags{
		configPath:        envOr("BACKREST_CONFIG", "./backrest.config.json"),
		dockerSocket:      envOr("DOCKER_HOST", "/var/run/docker.sock"),
		dockerRoot:        "/var/lib/docker",
		volumePrefix:      envOr("BACKREST_VOLUME_PREFIX", "/var/lib/docker/volumes"),
		defaultRepo:       "default",
		defaultSchedule:   "0 2 * * *",
		backrestContainer: "backrest",
		restartTimeout:    15 * time.Second,
		logFormat:         "json",
		logLevel:          "info",
	}

	rootCmd := &cobra.Command{
		Use:           "backrest-sidecar",
		Short:         "Backrest config sidecar for Docker Compose workloads",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// default command -> reconcile
			return runReconcile(cmd, flags)
		},
	}

	rootCmd.PersistentFlags().StringVar(&flags.logFormat, "log-format", flags.logFormat, "log format (json|text)")
	rootCmd.PersistentFlags().StringVar(&flags.logLevel, "log-level", flags.logLevel, "log level (debug|info|warn|error)")

	reconcileCmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Discover labeled containers and upsert Backrest plans",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReconcile(cmd, flags)
		},
	}
	bindReconcileFlags(reconcileCmd, &flags)

	daemonInterval := 60 * time.Second
	withEvents := false
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Continuous reconcile loop, optionally listening to docker events",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(cmd, flags, daemonInterval, withEvents)
		},
	}
	bindReconcileFlags(daemonCmd, &flags)
	daemonCmd.Flags().DurationVar(&daemonInterval, "interval", daemonInterval, "reconcile interval (e.g. 60s)")
	daemonCmd.Flags().BoolVar(&withEvents, "with-events", false, "subscribe to docker events for faster updates")

	backupOpts := newBackupCLIOptions()
	backupCmd := &cobra.Command{
		Use:   "backup-once",
		Short: "Run restic-compose-backup once plus per-workload retention",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackup(cmd, &flags, backupOpts)
		},
	}
	bindBackupFlags(backupCmd, &backupOpts)

	rootCmd.AddCommand(reconcileCmd, daemonCmd, backupCmd, newVersionCmd())
	return rootCmd
}

func bindReconcileFlags(cmd *cobra.Command, flags *commonFlags) {
	cmd.Flags().StringVar(&flags.configPath, "config", flags.configPath, "path to Backrest config file (defaults BACKREST_CONFIG)")
	cmd.Flags().BoolVar(&flags.apply, "apply", flags.apply, "restart Backrest container when config changes")
	cmd.Flags().StringVar(&flags.backrestContainer, "backrest-container", flags.backrestContainer, "container name/id for Backrest")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", flags.dryRun, "render plans but skip config write")
	cmd.Flags().StringVar(&flags.dockerSocket, "docker-sock", flags.dockerSocket, "docker socket path or host (e.g. /var/run/docker.sock)")
	cmd.Flags().StringVar(&flags.dockerRoot, "docker-root", flags.dockerRoot, "host docker root for named volumes")
	cmd.Flags().StringVar(&flags.volumePrefix, "volume-prefix", flags.volumePrefix, "rewrite derived volume sources to this prefix (e.g. /docker_volumes)")
	cmd.Flags().StringVar(&flags.defaultRepo, "default-repo", flags.defaultRepo, "fallback Backrest repo id")
	cmd.Flags().StringVar(&flags.defaultSchedule, "default-schedule", flags.defaultSchedule, "fallback cron schedule")
	cmd.Flags().BoolVar(&flags.excludeBindMounts, "exclude-bind-mounts", flags.excludeBindMounts, "derive sources only from named volumes")
	cmd.Flags().BoolVar(&flags.includeProjectName, "include-project-name", flags.includeProjectName, "prefix plan IDs with compose project")
	cmd.Flags().DurationVar(&flags.restartTimeout, "restart-timeout", flags.restartTimeout, "Backrest restart timeout")
}

func runReconcile(cmd *cobra.Command, flags commonFlags) error {
	logger, err := buildLogger(flags.logFormat, flags.logLevel)
	if err != nil {
		exitCode = 1
		return err
	}

	opts := app.ReconcileOptions{
		ConfigPath:         flags.configPath,
		Apply:              flags.apply,
		BackrestContainer:  flags.backrestContainer,
		DryRun:             flags.dryRun,
		DockerSocket:       flags.dockerSocket,
		DockerRoot:         flags.dockerRoot,
		VolumePrefix:       flags.volumePrefix,
		DefaultRepo:        flags.defaultRepo,
		DefaultSchedule:    flags.defaultSchedule,
		IncludeProjectName: flags.includeProjectName,
		ExcludeBindMounts:  flags.excludeBindMounts,
		Logger:             logger,
		RestartTimeout:     flags.restartTimeout,
	}

	reconciler, err := app.NewReconciler(opts)
	if err != nil {
		exitCode = 3
		return err
	}
	defer reconciler.Close()

	result, err := reconciler.Run(cmd.Context())
	if err != nil {
		logger.Error("reconcile.failed", slog.String("error", err.Error()))
		exitCode = 3
		return err
	}

	if !result.Changed || result.DryRun {
		exitCode = 2
	}
	return nil
}

func runDaemon(cmd *cobra.Command, flags commonFlags, interval time.Duration, withEvents bool) error {
	logger, err := buildLogger(flags.logFormat, flags.logLevel)
	if err != nil {
		exitCode = 1
		return err
	}
	opts := app.DaemonOptions{
		ReconcileOptions: app.ReconcileOptions{
			ConfigPath:         flags.configPath,
			Apply:              flags.apply,
			BackrestContainer:  flags.backrestContainer,
			DryRun:             flags.dryRun,
			DockerSocket:       flags.dockerSocket,
			DockerRoot:         flags.dockerRoot,
			VolumePrefix:       flags.volumePrefix,
			DefaultRepo:        flags.defaultRepo,
			DefaultSchedule:    flags.defaultSchedule,
			IncludeProjectName: flags.includeProjectName,
			ExcludeBindMounts:  flags.excludeBindMounts,
			Logger:             logger,
			RestartTimeout:     flags.restartTimeout,
		},
		Interval:   interval,
		WithEvents: withEvents,
	}
	if err := app.RunDaemon(cmd.Context(), opts); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		logger.Error("daemon.failed", slog.String("error", err.Error()))
		exitCode = 3
		return err
	}
	return nil
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version info",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "backrest-sidecar %s\n", version)
		},
	}
}

func buildLogger(format, level string) (*slog.Logger, error) {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %s", level)
	}

	opts := &slog.HandlerOptions{Level: slogLevel}
	var handler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		return nil, fmt.Errorf("invalid log format %s", format)
	}
	return slog.New(handler), nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type backupCLIOptions struct {
	rcbImage         string
	rcbEnvFile       string
	rcbCommand       []string
	rcbArgs          []string
	quiesceLabel     string
	quiesceTimeout   time.Duration
	resticGroupBy    string
	resticPathPrefix string
}

func newBackupCLIOptions() backupCLIOptions {
	return backupCLIOptions{
		rcbImage:         "zettaio/restic-compose-backup:0.7.1",
		rcbCommand:       []string{"rcb", "backup"},
		quiesceLabel:     "restic-compose-backup.quiesce=true",
		quiesceTimeout:   60 * time.Second,
		resticGroupBy:    "paths",
		resticPathPrefix: "/volumes",
	}
}

func bindBackupFlags(cmd *cobra.Command, opts *backupCLIOptions) {
	cmd.Flags().StringVar(&opts.rcbImage, "rcb-image", opts.rcbImage, "restic-compose-backup image reference")
	cmd.Flags().StringVar(&opts.rcbEnvFile, "rcb-env-file", opts.rcbEnvFile, "env file passed to rcb container")
	cmd.Flags().StringSliceVar(&opts.rcbCommand, "rcb-command", opts.rcbCommand, "rcb command + args (default: rcb backup)")
	cmd.Flags().StringSliceVar(&opts.rcbArgs, "rcb-arg", opts.rcbArgs, "additional args appended to rcb command")
	cmd.Flags().StringVar(&opts.quiesceLabel, "quiesce-label", opts.quiesceLabel, "label selector for sidecar-controlled quiesce")
	cmd.Flags().DurationVar(&opts.quiesceTimeout, "quiesce-timeout", opts.quiesceTimeout, "quiesce stop timeout")
	cmd.Flags().StringVar(&opts.resticGroupBy, "restic-group-by", opts.resticGroupBy, "restic --group-by value for retention")
	cmd.Flags().StringVar(&opts.resticPathPrefix, "restic-path-prefix", opts.resticPathPrefix, "base path prefix used in restic forget")
}

func runBackup(cmd *cobra.Command, flags *commonFlags, opts backupCLIOptions) error {
	logger, err := buildLogger(flags.logFormat, flags.logLevel)
	if err != nil {
		exitCode = 1
		return err
	}
	backupOpts := app.BackupOptions{
		DockerSocket:       flags.dockerSocket,
		DockerRoot:         flags.dockerRoot,
		IncludeProjectName: flags.includeProjectName,
		ExcludeBindMounts:  flags.excludeBindMounts,
		Logger:             logger,
		RCBImage:           opts.rcbImage,
		RCBCommand:         opts.rcbCommand,
		RCBEnvFile:         opts.rcbEnvFile,
		RCBExtraArgs:       opts.rcbArgs,
		QuiesceLabel:       opts.quiesceLabel,
		QuiesceTimeout:     opts.quiesceTimeout,
		ResticGroupBy:      opts.resticGroupBy,
		ResticPathPrefix:   opts.resticPathPrefix,
	}
	if err := app.RunBackup(cmd.Context(), backupOpts); err != nil {
		logger.Error("backup-once.failed", slog.String("error", err.Error()))
		exitCode = 3
		return err
	}
	return nil
}
