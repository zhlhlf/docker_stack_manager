package detector

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"docker_stack_manager/internal/db"
	dockerx "docker_stack_manager/internal/docker"
	"docker_stack_manager/internal/models"
)

const (
	ReasonNoStack        = "no_stack"
	ReasonPortNotAllowed = "port_not_allowed"
)

// Engine performs detection and cleanup.
type Engine struct {
	store  *db.Store
	docker *dockerx.Client
	mu     sync.Mutex
}

// New creates a detector engine.
func New(store *db.Store, dockerClient *dockerx.Client) *Engine {
	return &Engine{store: store, docker: dockerClient}
}

// Detect evaluates all services against configured stacks.
func (e *Engine) Detect(ctx context.Context) ([]models.ServiceInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.detectLocked(ctx, false)
}

// Clean detects and removes violating services.
func (e *Engine) Clean(ctx context.Context) (cleaned []models.ServiceInfo, all []models.ServiceInfo, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	all, err = e.detectLocked(ctx, true)
	if err != nil {
		return nil, nil, err
	}

	for _, svc := range all {
		if !svc.Violation.IsViolation {
			continue
		}
		if remErr := e.docker.RemoveService(ctx, svc.ID); remErr != nil {
			return cleaned, all, fmt.Errorf("remove service %s: %w", svc.Name, remErr)
		}
		_ = e.store.MarkViolationsCleaned(svc.Name)
		cleaned = append(cleaned, svc)
	}

	_ = e.store.SetSetting("last_clean_time", time.Now().UTC().Format(time.RFC3339))
	if cleaned == nil {
		cleaned = []models.ServiceInfo{}
	}
	return cleaned, all, nil
}

func (e *Engine) detectLocked(ctx context.Context, logViolations bool) ([]models.ServiceInfo, error) {
	stacks, err := e.store.ListStacks()
	if err != nil {
		return nil, err
	}
	stackMap := map[string]models.Stack{}
	for _, st := range stacks {
		stackMap[st.Name] = st
	}

	services, err := e.docker.ListServices(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]models.ServiceInfo, 0, len(services))
	for _, svc := range services {
		info := models.ServiceInfo{
			ID:             svc.ID,
			Name:           svc.Name,
			Stack:          resolveStack(svc, stackMap),
			PublishedPorts: publishedPortStrings(svc.PublishedPorts),
			Violation:      models.ViolationInfo{},
		}

		reason := evaluateViolation(info, svc.PublishedPorts, stackMap)
		if reason != "" {
			info.Violation = models.ViolationInfo{IsViolation: true, Reason: reason}
			if logViolations {
				_ = e.store.AddViolationLog(info.Name, info.Stack, reason)
			}
		}
		out = append(out, info)
	}
	return out, nil
}

func resolveStack(svc dockerx.ServiceView, stackMap map[string]models.Stack) string {
	if svc.StackLabel != "" {
		return svc.StackLabel
	}
	if idx := strings.Index(svc.Name, "_"); idx > 0 {
		prefix := svc.Name[:idx]
		if _, ok := stackMap[prefix]; ok {
			return prefix
		}
	}
	return ""
}

func evaluateViolation(info models.ServiceInfo, ports []dockerx.PublishedPort, stackMap map[string]models.Stack) string {
	if info.Stack == "" {
		return ReasonNoStack
	}
	st, ok := stackMap[info.Stack]
	if !ok {
		return ReasonNoStack
	}
	if len(ports) == 0 {
		return ""
	}
	if len(st.Ports) == 0 {
		return ReasonPortNotAllowed
	}
	for _, p := range ports {
		if !portAllowed(p, st.Ports) {
			return ReasonPortNotAllowed
		}
	}
	return ""
}

func portAllowed(p dockerx.PublishedPort, allowed []models.Port) bool {
	for _, a := range allowed {
		proto := strings.ToLower(a.Protocol)
		if proto == "" {
			proto = "tcp"
		}
		if proto != strings.ToLower(p.Protocol) {
			continue
		}
		if matchPort(a.Port, p.PublishedPort) {
			return true
		}
	}
	return false
}

func matchPort(spec string, published uint32) bool {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return false
	}
	if strings.Contains(spec, "-") {
		parts := strings.SplitN(spec, "-", 2)
		if len(parts) != 2 {
			return false
		}
		start, err1 := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 32)
		end, err2 := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 32)
		if err1 != nil || err2 != nil {
			return false
		}
		if start > end {
			start, end = end, start
		}
		v := uint64(published)
		return v >= start && v <= end
	}
	exact, err := strconv.ParseUint(spec, 10, 32)
	if err != nil {
		return false
	}
	return uint64(published) == exact
}

func publishedPortStrings(ports []dockerx.PublishedPort) []string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, fmt.Sprintf("%d/%s", p.PublishedPort, p.Protocol))
	}
	return out
}

// Stats builds dashboard stats.
func (e *Engine) Stats(ctx context.Context) (*models.DashboardStats, error) {
	stackCount, err := e.store.CountStacks()
	if err != nil {
		return nil, err
	}
	services, err := e.Detect(ctx)
	if err != nil {
		auto, _ := e.store.GetSetting("auto_clean_enabled")
		return &models.DashboardStats{
			StackCount:       stackCount,
			ServiceCount:     0,
			ViolationCount:   0,
			AutoCleanEnabled: auto == "true",
		}, err
	}
	violations := 0
	for _, s := range services {
		if s.Violation.IsViolation {
			violations++
		}
	}
	auto, _ := e.store.GetSetting("auto_clean_enabled")
	return &models.DashboardStats{
		StackCount:       stackCount,
		ServiceCount:     len(services),
		ViolationCount:   violations,
		AutoCleanEnabled: auto == "true",
	}, nil
}