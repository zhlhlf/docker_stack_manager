package detector

import (
	"context"
	"fmt"
	"sort"
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
	if svc.StackLabel != "" {
		return svc.StackLabel
	}
	return ""
}

// matchStackByPrefix picks the longest configured stack name that is a prefix of serviceName.
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

func serviceBelongsToStack(svc dockerx.ServiceView, stackName string) bool {
	if stackName == "" {
		return false
	}
	if svc.StackLabel == stackName {
		return true
	}
	// Parent unit matches child docker stack labels:
	// stackName=csc-preview-bank, label=csc-preview-bank-bf
	if svc.StackLabel != "" && strings.HasPrefix(svc.StackLabel, stackName) {
		if len(svc.StackLabel) == len(stackName) {
			return true
		}
		ch := svc.StackLabel[len(stackName)]
		if ch == '-' || ch == '_' || ch == '.' {
			return true
		}
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

func suggestStackName(svc dockerx.ServiceView, resolved string, stackMap map[string]models.Stack) string {
	if resolved != "" {
		return resolved
	}
	if svc.StackLabel != "" {
		return svc.StackLabel
	}
	return ""
}

type violationRow struct {
	svc    dockerx.ServiceView
	reason string
	stack  string
}

// ListViolationStacks groups current violating services by stack name.
// Rules for stack unit name:
//  1) resolved/configured stack or docker stack label (initial)
//  2) services on the same network whose stack labels/names share a common
//     boundary-aware prefix are merged into that prefix unit
//     e.g. labels csc-preview-bank-bf / csc-preview-bank-czt / csc-preview-bank-dp
//          on one network => unit "csc-preview-bank"
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

	var viols []violationRow
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
		viols = append(viols, violationRow{svc: svc, reason: reason, stack: name})
	}

	// Fill empty names first via network+service-name prefix.
	orphans := make([]violationRow, 0)
	named := make([]violationRow, 0)
	for _, v := range viols {
		if strings.TrimSpace(v.stack) == "" {
			orphans = append(orphans, v)
		} else {
			named = append(named, v)
		}
	}
	viols = append(named, inferStackUnitsByNetworkPrefix(orphans)...)

	// Merge different labels that share network + common prefix into one unit.
	viols = mergeSameNetworkCommonPrefix(viols)

	type acc struct {
		services map[string]struct{}
		ports    map[string]struct{}
		reasons  map[string]struct{}
	}
	groups := map[string]*acc{}
	for _, v := range viols {
		name := strings.TrimSpace(v.stack)
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
		g.services[v.svc.Name] = struct{}{}
		g.reasons[v.reason] = struct{}{}
		for _, p := range publishedPortStrings(v.svc.PublishedPorts) {
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

// mergeSameNetworkCommonPrefix rewrites stack names for violators that share at
// least one network and whose current stack names share a boundary-aware prefix.
func mergeSameNetworkCommonPrefix(rows []violationRow) []violationRow {
	if len(rows) < 2 {
		return rows
	}

	// network -> row indexes
	byNet := map[string][]int{}
	for i, v := range rows {
		nets := v.svc.Networks
		if len(nets) == 0 {
			continue
		}
		for _, n := range nets {
			byNet[n] = append(byNet[n], i)
		}
	}

	assigned := make([]string, len(rows))
	for i := range rows {
		assigned[i] = rows[i].stack
	}

	for _, idxs := range byNet {
		if len(idxs) < 2 {
			continue
		}
		// unique stack names currently used by this network group
		nameSet := map[string]struct{}{}
		var names []string
		for _, i := range idxs {
			n := strings.TrimSpace(assigned[i])
			if n == "" {
				// fallback to service name for prefix calc
				n = rows[i].svc.Name
			}
			if _, ok := nameSet[n]; ok {
				continue
			}
			nameSet[n] = struct{}{}
			names = append(names, n)
		}
		if len(names) < 2 {
			continue
		}
		prefix := longestCommonBoundaryPrefix(names)
		if prefix == "" {
			continue
		}
		// only merge if prefix is strictly shorter than at least one name
		// (otherwise they are already the same unit)
		shorter := false
		for _, n := range names {
			if n != prefix {
				shorter = true
				break
			}
		}
		if !shorter {
			continue
		}
		for _, i := range idxs {
			// Prefer longer unit already assigned from another net only if longer.
			if assigned[i] == "" || len(prefix) < len(assigned[i]) || !strings.HasPrefix(assigned[i], prefix) {
				// If current name already starts with prefix, collapse to prefix.
				cur := assigned[i]
				if cur == "" || strings.HasPrefix(cur, prefix) {
					assigned[i] = prefix
				}
			}
			// Always collapse when current stack/label is a child of prefix.
			if assigned[i] != prefix && strings.HasPrefix(assigned[i], prefix) {
				nextOK := false
				if len(assigned[i]) == len(prefix) {
					nextOK = true
				} else {
					ch := assigned[i][len(prefix)]
					nextOK = ch == '-' || ch == '_' || ch == '.'
				}
				if nextOK {
					assigned[i] = prefix
				}
			}
		}
	}

	out := make([]violationRow, len(rows))
	for i, v := range rows {
		v.stack = assigned[i]
		out[i] = v
	}
	return out
}
func inferStackUnitsByNetworkPrefix(orphans []violationRow) []violationRow {
	if len(orphans) == 0 {
		return nil
	}
	byNet := map[string][]int{}
	for i, v := range orphans {
		nets := v.svc.Networks
		if len(nets) == 0 {
			nets = []string{"_none_"}
		}
		for _, n := range nets {
			byNet[n] = append(byNet[n], i)
		}
	}

	assigned := make([]string, len(orphans))
	for _, idxs := range byNet {
		if len(idxs) < 2 {
			if len(idxs) == 1 {
				name := weakPrefixUnit(orphans[idxs[0]].svc.Name)
				if name != "" && assigned[idxs[0]] == "" {
					assigned[idxs[0]] = name
				}
			}
			continue
		}
		names := make([]string, 0, len(idxs))
		for _, i := range idxs {
			names = append(names, orphans[i].svc.Name)
		}
		prefix := longestCommonBoundaryPrefix(names)
		if prefix == "" {
			continue
		}
		for _, i := range idxs {
			if assigned[i] == "" || len(prefix) > len(assigned[i]) {
				assigned[i] = prefix
			}
		}
	}

	out := make([]violationRow, 0, len(orphans))
	for i, v := range orphans {
		v.stack = assigned[i]
		out = append(out, v)
	}
	return out
}

func weakPrefixUnit(name string) string {
	for _, sep := range []string{"-", "_"} {
		parts := strings.Split(name, sep)
		if len(parts) >= 2 && parts[0] != "" {
			return parts[0]
		}
	}
	return ""
}

func longestCommonBoundaryPrefix(names []string) string {
	if len(names) == 0 {
		return ""
	}
	prefix := names[0]
	for _, n := range names[1:] {
		prefix = commonPrefix(prefix, n)
		if prefix == "" {
			return ""
		}
	}
	cut := -1
	for i := 0; i < len(prefix); i++ {
		if prefix[i] == '-' || prefix[i] == '_' || prefix[i] == '.' {
			cut = i
		}
	}
	if cut <= 0 {
		for _, n := range names {
			if n != prefix {
				return ""
			}
		}
		return prefix
	}
	if len(prefix) < len(names[0]) {
		unit := prefix[:cut]
		if unit == "" {
			return ""
		}
		for _, n := range names {
			if !strings.HasPrefix(n, unit) {
				return ""
			}
			if len(n) == len(unit) {
				continue
			}
			next := n[len(unit)]
			if next != '-' && next != '_' && next != '.' {
				return ""
			}
		}
		return unit
	}
	return strings.TrimRight(prefix, "-_.")
}

func commonPrefix(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
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