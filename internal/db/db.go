package db

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"docker_stack_manager/internal/models"
)

// Store is a JSON-backed data store (stdlib only).
type Store struct {
	mu   sync.RWMutex
	path string
	data *persist
}

type persist struct {
	NextStackID int64                    `json:"next_stack_id"`
	NextPortID  int64                    `json:"next_port_id"`
	NextLogID   int64                    `json:"next_log_id"`
	Stacks      []models.Stack           `json:"stacks"`
	Ports       []models.Port            `json:"ports"`
	Settings    map[string]string        `json:"settings"`
	Logs        []models.ViolationLog    `json:"logs"`
}

// New opens or creates the JSON store.
func New(path string) (*Store, error) {
	s := &Store{
		path: path,
		data: &persist{
			NextStackID: 1,
			NextPortID:  1,
			NextLogID:   1,
			Stacks:      []models.Stack{},
			Ports:       []models.Port{},
			Settings: map[string]string{
				"auto_clean_enabled": "false",
				"clean_interval":     "300",
				"last_clean_time":    "",
				"dingtalk_webhook":    "",
			},
			Logs: []models.ViolationLog{},
		},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s.saveLocked()
		}
		return err
	}
	if len(b) == 0 {
		return s.saveLocked()
	}
	var p persist
	if err := json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("parse db: %w", err)
	}
	if p.Settings == nil {
		p.Settings = map[string]string{}
	}
	if _, ok := p.Settings["auto_clean_enabled"]; !ok {
		p.Settings["auto_clean_enabled"] = "false"
	}
	if _, ok := p.Settings["clean_interval"]; !ok {
		p.Settings["clean_interval"] = "300"
	}
	if _, ok := p.Settings["last_clean_time"]; !ok {
		p.Settings["last_clean_time"] = ""
	}
	if _, ok := p.Settings["dingtalk_webhook"]; !ok {
		p.Settings["dingtalk_webhook"] = ""
	}
	if p.NextStackID < 1 {
		p.NextStackID = 1
	}
	if p.NextPortID < 1 {
		p.NextPortID = 1
	}
	if p.NextLogID < 1 {
		p.NextLogID = 1
	}
	if p.Stacks == nil {
		p.Stacks = []models.Stack{}
	}
	if p.Ports == nil {
		p.Ports = []models.Port{}
	}
	if p.Logs == nil {
		p.Logs = []models.ViolationLog{}
	}
	s.data = &p
	return nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

// Close flushes data.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

// ListStacks returns stacks with ports.
func (s *Store) ListStacks() ([]models.Stack, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Stack, 0, len(s.data.Stacks))
	for _, st := range s.data.Stacks {
		st.Ports = s.portsOf(st.ID)
		out = append(out, st)
	}
	return out, nil
}

func (s *Store) portsOf(stackID int64) []models.Port {
	ports := make([]models.Port, 0)
	for _, p := range s.data.Ports {
		if p.StackID == stackID {
			ports = append(ports, p)
		}
	}
	return ports
}

// GetStack returns one stack.
func (s *Store) GetStack(id int64) (*models.Stack, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, st := range s.data.Stacks {
		if st.ID == id {
			cp := st
			cp.Ports = s.portsOf(id)
			return &cp, nil
		}
	}
	return nil, nil
}

// GetStackByName returns stack by name.
func (s *Store) GetStackByName(name string) (*models.Stack, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, st := range s.data.Stacks {
		if st.Name == name {
			cp := st
			cp.Ports = s.portsOf(st.ID)
			return &cp, nil
		}
	}
	return nil, nil
}

// CreateStack creates a stack.
func (s *Store) CreateStack(name, description string) (*models.Stack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("stack name is required")
	}
	for _, st := range s.data.Stacks {
		if st.Name == name {
			return nil, fmt.Errorf("stack already exists")
		}
	}
	st := models.Stack{
		ID:          s.data.NextStackID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().UTC(),
		Ports:       []models.Port{},
	}
	s.data.NextStackID++
	s.data.Stacks = append(s.data.Stacks, st)
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return &st, nil
}

// UpdateStack updates description.
func (s *Store) UpdateStack(id int64, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Stacks {
		if s.data.Stacks[i].ID == id {
			s.data.Stacks[i].Description = description
			return s.saveLocked()
		}
	}
	return fmt.Errorf("stack not found")
}

// DeleteStack deletes stack and ports.
func (s *Store) DeleteStack(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	ns := make([]models.Stack, 0, len(s.data.Stacks))
	for _, st := range s.data.Stacks {
		if st.ID == id {
			found = true
			continue
		}
		ns = append(ns, st)
	}
	if !found {
		return fmt.Errorf("stack not found")
	}
	s.data.Stacks = ns
	np := make([]models.Port, 0, len(s.data.Ports))
	for _, p := range s.data.Ports {
		if p.StackID == id {
			continue
		}
		np = append(np, p)
	}
	s.data.Ports = np
	return s.saveLocked()
}

// ListPorts lists ports of a stack.
func (s *Store) ListPorts(stackID int64) ([]models.Port, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.portsOf(stackID), nil
}

// AddPort adds a port whitelist entry.
func (s *Store) AddPort(stackID int64, port, protocol string) (*models.Port, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	port = strings.TrimSpace(port)
	if port == "" {
		return nil, fmt.Errorf("port is required")
	}
	if protocol == "" {
		protocol = "tcp"
	}
	protocol = strings.ToLower(protocol)
	if protocol != "tcp" && protocol != "udp" {
		return nil, fmt.Errorf("protocol must be tcp or udp")
	}
	exists := false
	for _, st := range s.data.Stacks {
		if st.ID == stackID {
			exists = true
			break
		}
	}
	if !exists {
		return nil, fmt.Errorf("stack not found")
	}
	p := models.Port{
		ID:       s.data.NextPortID,
		StackID:  stackID,
		Port:     port,
		Protocol: protocol,
	}
	s.data.NextPortID++
	s.data.Ports = append(s.data.Ports, p)
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return &p, nil
}

// DeletePort deletes a port entry.
func (s *Store) DeletePort(portID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	np := make([]models.Port, 0, len(s.data.Ports))
	for _, p := range s.data.Ports {
		if p.ID == portID {
			found = true
			continue
		}
		np = append(np, p)
	}
	if !found {
		return fmt.Errorf("port not found")
	}
	s.data.Ports = np
	return s.saveLocked()
}

// GetSettings returns all settings.
func (s *Store) GetSettings() (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]string{}
	for k, v := range s.data.Settings {
		out[k] = v
	}
	return out, nil
}

// UpdateSettings updates settings map.
func (s *Store) UpdateSettings(values map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range values {
		s.data.Settings[k] = v
	}
	return s.saveLocked()
}

// GetSetting returns one setting.
func (s *Store) GetSetting(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Settings[key], nil
}

// SetSetting sets one setting.
func (s *Store) SetSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Settings[key] = value
	return s.saveLocked()
}

// AddViolationLog appends a violation log.
func (s *Store) AddViolationLog(serviceName, stackName, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Logs = append(s.data.Logs, models.ViolationLog{
		ID:          s.data.NextLogID,
		ServiceName: serviceName,
		StackName:   stackName,
		Reason:      reason,
		DetectedAt:  time.Now().UTC(),
		Cleaned:     false,
	})
	s.data.NextLogID++
	// keep last 500
	if len(s.data.Logs) > 500 {
		s.data.Logs = s.data.Logs[len(s.data.Logs)-500:]
	}
	return s.saveLocked()
}

// MarkViolationsCleaned marks service logs cleaned.
func (s *Store) MarkViolationsCleaned(serviceName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Logs {
		if s.data.Logs[i].ServiceName == serviceName && !s.data.Logs[i].Cleaned {
			s.data.Logs[i].Cleaned = true
		}
	}
	return s.saveLocked()
}

// ListViolationLogs returns recent logs newest first.
func (s *Store) ListViolationLogs(limit int) ([]models.ViolationLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	n := len(s.data.Logs)
	out := make([]models.ViolationLog, 0, limit)
	for i := n - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.data.Logs[i])
	}
	return out, nil
}

// CountStacks returns stack count.
func (s *Store) CountStacks() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data.Stacks), nil
}

// HasPort reports whether stack already has the port/protocol.
func (s *Store) HasPort(stackID int64, port, protocol string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	protocol = strings.ToLower(protocol)
	if protocol == "" {
		protocol = "tcp"
	}
	for _, p := range s.data.Ports {
		if p.StackID == stackID && p.Port == port && strings.ToLower(p.Protocol) == protocol {
			return true, nil
		}
	}
	return false, nil
}
