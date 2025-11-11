package executil

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// RunOptions controls how Run executes an external command.
type RunOptions struct {
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
	Dir    string
}

// Run executes the command with the provided context and options.
func Run(ctx context.Context, name string, args []string, opts RunOptions) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = os.Stdout
	}
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}
