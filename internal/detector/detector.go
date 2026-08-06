package detector

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sort"
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
	return e.detectLocked(ctx, true)
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

	sort.SliceStable(out, func(i, j int) bool {
		si, sj := out[i].Stack, out[j].Stack
		// empty stack last
		if si == "" && sj != "" {
			return false
		}
		if si != "" && sj == "" {
			return true
		}
		if si != sj {
			return si < sj
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// resolveStack decides which configured stack a service belongs to.
// Priority:
//  1) Docker label com.docker.stack.namespace if it matches a configured stack
//  2) Longest configured stack-name prefix of the service name
//     e.g. service "czt-zhongtoubao-czt" + stack "czt-zhongtoubao" => match
//  3) Label value even if not configured (will be treated as no_stack later)
func resolveStack(svc dockerx.ServiceView, stackMap map[string]models.Stack) string {
	if svc.StackLabel != "" {
		if _, ok := stackMap[svc.StackLabel]; ok {
			return svc.StackLabel
		}
	}
	if name := matchStackByPrefix(svc.Name, stackMap); name != "" {
		return name
	}
	// Unconfigured label still returned for display; evaluateViolation marks no_stack.
	if svc.StackLabel != "" {
		return svc.StackLabel
	}
	return ""
}

// matchStackByPrefix picks the longest configured stack name that is a prefix of serviceName.
// If serviceName is longer than the stack, the next character must be a separator (- _ .).
func matchStackByPrefix(serviceName string, stackMap map[string]models.Stack) string {
	best := ""
	for name := range stackMap {
		if !strings.HasPrefix(serviceName, name) {
			continue
		}
		if len(serviceName) == len(name) {
			if len(name) > len(best) {
				best = name
			}
			continue
		}
		// require boundary so "web" does not claim "webapp"
		next := serviceName[len(name)]
		if next == '-' || next == '_' || next == '.' {
			if len(name) > len(best) {
				best = name
			}
		}
	}
	return best
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

// serviceBelongsToStack reports whether a raw docker service belongs to stackName
// by label or name prefix boundary match.
func serviceBelongsToStack(svc dockerx.ServiceView, stackName string) bool {
	if stackName == "" {
		return false
	}
	if svc.StackLabel == stackName {
		return true
	}
	if !strings.HasPrefix(svc.Name, stackName) {
		return false
	}
	if len(svc.Name) == len(stackName) {
		return true
	}
	next := svc.Name[len(stackName)]
	return next == '-' || next == '_' || next == '.'
}

// suggestStackName picks a stack name for a violating service.
func suggestStackName(svc dockerx.ServiceView, resolved string, stackMap map[string]models.Stack) string {
	if resolved != "" {
		return resolved
	}
	if svc.StackLabel != "" {
		return svc.StackLabel
	}
	// No reliable stack name for pure orphans without label.
	return ""
}

// ListViolationStacks groups current violating services by stack name.
func (e *Engine) ListViolationStacks(ctx context.Context) ([]models.ViolationStack, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

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

	type acc struct {
		services map[string]struct{}
		ports    map[string]struct{}
		reasons  map[string]struct{}
	}
	groups := map[string]*acc{}

	for _, svc := range services {
		info := models.ServiceInfo{
			ID:             svc.ID,
			Name:           svc.Name,
			Stack:          resolveStack(svc, stackMap),
			PublishedPorts: publishedPortStrings(svc.PublishedPorts),
		}
		reason := evaluateViolation(info, svc.PublishedPorts, stackMap)
		if reason == "" {
			continue
		}
		name := suggestStackName(svc, info.Stack, stackMap)
		if name == "" {
			continue
		}
		g, ok := groups[name]
		if !ok {
			g = &acc{
				services: map[string]struct{}{},
				ports:    map[string]struct{}{},
				reasons:  map[string]struct{}{},
			}
			groups[name] = g
		}
		g.services[svc.Name] = struct{}{}
		g.reasons[reason] = struct{}{}
		for _, p := range publishedPortStrings(svc.PublishedPorts) {
			g.ports[p] = struct{}{}
		}
	}

	out := make([]models.ViolationStack, 0, len(groups))
	for name, g := range groups {
		item := models.ViolationStack{
			Name:         name,
			ServiceCount: len(g.services),
			Services:     make([]string, 0, len(g.services)),
			Ports:        make([]string, 0, len(g.ports)),
			Reasons:      make([]string, 0, len(g.reasons)),
			Configured:   false,
		}
		if _, ok := stackMap[name]; ok {
			item.Configured = true
		}
		for s := range g.services {
			item.Services = append(item.Services, s)
		}
		for p := range g.ports {
			item.Ports = append(item.Ports, p)
		}
		for r := range g.reasons {
			item.Reasons = append(item.Reasons, r)
		}
		sort.Strings(item.Services)
		sort.Strings(item.Ports)
		sort.Strings(item.Reasons)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// WhitelistStack creates/ensures stack and adds published ports from matching running services.
func (e *Engine) WhitelistStack(ctx context.Context, name, description string) (*models.WhitelistStackResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("stack name is required")
	}

	created := false
	st, err := e.store.GetStackByName(name)
	if err != nil {
		return nil, err
	}
	if st == nil {
		st, err = e.store.CreateStack(name, description)
		if err != nil {
			return nil, err
		}
		created = true
	}

	raw, err := e.docker.ListServices(ctx)
	if err != nil {
		return nil, err
	}

	result := &models.WhitelistStackResult{
		Created:         created,
		AddedPorts:      []string{},
		SkippedPorts:    []string{},
		MatchedServices: []string{},
	}

	seenPort := map[string]struct{}{}
	for _, svc := range raw {
		if !serviceBelongsToStack(svc, name) {
			continue
		}
		result.MatchedServices = append(result.MatchedServices, svc.Name)
		for _, p := range svc.PublishedPorts {
			key := fmt.Sprintf("%d/%s", p.PublishedPort, p.Protocol)
			if _, ok := seenPort[key]; ok {
				continue
			}
			seenPort[key] = struct{}{}
			portStr := strconv.FormatUint(uint64(p.PublishedPort), 10)
			proto := p.Protocol
			if proto == "" {
				proto = "tcp"
			}
			exists, err := e.store.HasPort(st.ID, portStr, proto)
			if err != nil {
				return nil, err
			}
			if exists {
				result.SkippedPorts = append(result.SkippedPorts, key)
				continue
			}
			if _, err := e.store.AddPort(st.ID, portStr, proto); err != nil {
				return nil, err
			}
			result.AddedPorts = append(result.AddedPorts, key)
		}
	}
	sort.Strings(result.MatchedServices)
	sort.Strings(result.AddedPorts)
	sort.Strings(result.SkippedPorts)

	st, _ = e.store.GetStack(st.ID)
	result.Stack = st
	return result, nil
}