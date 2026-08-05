package dockerx

import (
	"context"
	"sync"
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

// Client wraps Docker access. Current build uses an in-memory mock so the
// project compiles with pure Go stdlib (no external modules required).
// Swap ListServices/RemoveService with real Docker Engine SDK calls for production.
type Client struct {
	mu       sync.RWMutex
	services []ServiceView
}

// New creates a client with demo swarm services.
func New() (*Client, error) {
	return &Client{
		services: []ServiceView{
			{
				ID:         "svc-webapp-api",
				Name:       "webapp_api",
				StackLabel: "webapp",
				PublishedPorts: []PublishedPort{
					{PublishedPort: 8080, TargetPort: 8080, Protocol: "tcp"},
				},
			},
			{
				ID:         "svc-webapp-bad",
				Name:       "webapp_admin",
				StackLabel: "webapp",
				PublishedPorts: []PublishedPort{
					{PublishedPort: 9999, TargetPort: 9999, Protocol: "tcp"},
				},
			},
			{
				ID:         "svc-db",
				Name:       "db_mysql",
				StackLabel: "db",
				PublishedPorts: []PublishedPort{
					{PublishedPort: 3306, TargetPort: 3306, Protocol: "tcp"},
				},
			},
			{
				ID:         "svc-orphan",
				Name:       "legacy_app",
				StackLabel: "",
				PublishedPorts: []PublishedPort{
					{PublishedPort: 3000, TargetPort: 3000, Protocol: "tcp"},
				},
			},
			{
				ID:             "svc-noports",
				Name:           "worker_job",
				StackLabel:     "webapp",
				PublishedPorts: nil,
			},
		},
	}, nil
}

// Close closes the client.
func (c *Client) Close() error { return nil }

// Ping checks availability.
func (c *Client) Ping(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// ListServices returns current services.
func (c *Client) ListServices(ctx context.Context) ([]ServiceView, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]ServiceView, len(c.services))
	copy(out, c.services)
	return out, nil
}

// RemoveService removes a service by id or name.
func (c *Client) RemoveService(ctx context.Context, idOrName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := c.services[:0]
	for _, svc := range c.services {
		if svc.ID == idOrName || svc.Name == idOrName {
			continue
		}
		next = append(next, svc)
	}
	c.services = next
	return nil
}