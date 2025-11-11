package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerevents "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// Options configures the Docker client creation.
type Options struct {
	Host       string
	APIVersion string
}

// Client wraps the Docker API client.
type Client struct {
	cli *client.Client
}

// New creates a new Docker client using the provided options.
func New(opts Options) (*Client, error) {
	clientOpts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}
	if opts.Host != "" {
		clientOpts = append(clientOpts, client.WithHost(opts.Host))
	}
	if opts.APIVersion != "" {
		clientOpts = append(clientOpts, client.WithVersion(opts.APIVersion))
	}

	cli, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli}, nil
}

// Close releases underlying resources.
func (c *Client) Close() error {
	if c == nil || c.cli == nil {
		return nil
	}
	return c.cli.Close()
}

// Container holds the subset of metadata required by the sidecar.
type Container struct {
	ID      string
	Name    string
	Labels  map[string]string
	Mounts  []dockertypes.MountPoint
	Project string
	Service string
	State   string
	Status  string
}

// ListBackrestEnabled finds containers opt-in via labels.
func (c *Client) ListBackrestEnabled(ctx context.Context) ([]Container, error) {
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", "backrest.enable=true")

	list, err := c.cli.ContainerList(ctx, dockertypes.ContainerListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, err
	}

	containers := make([]Container, 0, len(list))
	for _, ctr := range list {
		name := strings.TrimPrefix(first(ctr.Names), "/")
		containers = append(containers, Container{
			ID:      ctr.ID,
			Name:    name,
			Labels:  ctr.Labels,
			Mounts:  ctr.Mounts,
			Project: ctr.Labels["com.docker.compose.project"],
			Service: ctr.Labels["com.docker.compose.service"],
			State:   ctr.State,
			Status:  ctr.Status,
		})
	}
	return containers, nil
}

// ListByLabel returns containers matching an arbitrary label selector (key=value).
func (c *Client) ListByLabel(ctx context.Context, selector string) ([]Container, error) {
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", selector)

	list, err := c.cli.ContainerList(ctx, dockertypes.ContainerListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, err
	}

	containers := make([]Container, 0, len(list))
	for _, ctr := range list {
		name := strings.TrimPrefix(first(ctr.Names), "/")
		containers = append(containers, Container{
			ID:      ctr.ID,
			Name:    name,
			Labels:  ctr.Labels,
			Mounts:  ctr.Mounts,
			Project: ctr.Labels["com.docker.compose.project"],
			Service: ctr.Labels["com.docker.compose.service"],
			State:   ctr.State,
			Status:  ctr.Status,
		})
	}
	return containers, nil
}

// RestartContainer restarts the container name/ID.
func (c *Client) RestartContainer(ctx context.Context, name string, timeout time.Duration) error {
	if name == "" {
		return errors.New("container name required")
	}
	return c.cli.ContainerRestart(ctx, name, &timeout)
}

// StopContainer stops the container with timeout.
func (c *Client) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	t := timeout
	return c.cli.ContainerStop(ctx, id, container.StopOptions{
		Timeout: &t,
	})
}

// StartContainer starts a container.
func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.cli.ContainerStart(ctx, id, dockertypes.ContainerStartOptions{})
}

// Events subscribes to Docker events with the provided filters.
func (c *Client) Events(ctx context.Context, filter filters.Args) (<-chan dockerevents.Message, <-chan error) {
	return c.cli.Events(ctx, dockertypes.EventsOptions{
		Filters: filter,
	})
}

func first(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}
