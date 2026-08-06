package dockerx

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
)

// ServiceView is a simplified Docker service representation.
type ServiceView struct {
	ID             string
	Name           string
	StackLabel     string
	PublishedPorts []PublishedPort
}

// PublishedPort is a published service port.
type PublishedPort struct {
	PublishedPort uint32
	TargetPort    uint32
	Protocol      string
}

// Client wraps Docker Engine API access for Swarm services.
type Client struct {
	cli *client.Client
}

// New creates a Docker client from environment (DOCKER_HOST / default pipe or socket).
func New() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Client{cli: cli}, nil
}

// Close closes the Docker client.
func (c *Client) Close() error {
	if c == nil || c.cli == nil {
		return nil
	}
	return c.cli.Close()
}

// Ping checks Docker Engine availability.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.cli == nil {
		return fmt.Errorf("docker client is nil")
	}
	_, err := c.cli.Ping(ctx)
	return err
}

// Info returns brief engine info for diagnostics.
func (c *Client) Info(ctx context.Context) (string, error) {
	info, err := c.cli.Info(ctx)
	if err != nil {
		return "", err
	}
	swarmState := string(info.Swarm.LocalNodeState)
	if swarmState == "" {
		swarmState = "inactive"
	}
	return fmt.Sprintf("name=%s swarm=%s", info.Name, swarmState), nil
}

// ListServices returns all swarm services.
func (c *Client) ListServices(ctx context.Context) ([]ServiceView, error) {
	if c == nil || c.cli == nil {
		return nil, fmt.Errorf("docker client is nil")
	}
	services, err := c.cli.ServiceList(ctx, types.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ServiceList: %w", err)
	}
	out := make([]ServiceView, 0, len(services))
	for _, svc := range services {
		out = append(out, toServiceView(svc))
	}
	return out, nil
}

// RemoveService removes a service by ID or name.
// Missing services are treated as success.
func (c *Client) RemoveService(ctx context.Context, idOrName string) error {
	if c == nil || c.cli == nil {
		return fmt.Errorf("docker client is nil")
	}
	err := c.cli.ServiceRemove(ctx, idOrName)
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "not found") || strings.Contains(msg, "no such service") {
		return nil
	}
	return fmt.Errorf("ServiceRemove %s: %w", idOrName, err)
}

func toServiceView(svc swarm.Service) ServiceView {
	view := ServiceView{
		ID:   svc.ID,
		Name: svc.Spec.Name,
	}
	if svc.Spec.Labels != nil {
		view.StackLabel = svc.Spec.Labels["com.docker.stack.namespace"]
	}

	// Prefer runtime published ports, fallback to endpoint spec.
	ports := svc.Endpoint.Ports
	if len(ports) == 0 && svc.Spec.EndpointSpec != nil {
		ports = svc.Spec.EndpointSpec.Ports
	}
	for _, p := range ports {
		proto := strings.ToLower(string(p.Protocol))
		if proto == "" {
			proto = "tcp"
		}
		view.PublishedPorts = append(view.PublishedPorts, PublishedPort{
			PublishedPort: p.PublishedPort,
			TargetPort:    p.TargetPort,
			Protocol:      proto,
		})
	}
	return view
}