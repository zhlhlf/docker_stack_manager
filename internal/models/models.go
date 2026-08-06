package models

import "time"

// Stack represents a Docker stack definition.
type Stack struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	Ports       []Port    `json:"ports,omitempty"`
}

// Port is an allowed port or port range for a stack.
type Port struct {
	ID       int64  `json:"id"`
	StackID  int64  `json:"stack_id"`
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
}

// ViolationLog records a detected violation.
type ViolationLog struct {
	ID          int64     `json:"id"`
	ServiceName string    `json:"service_name"`
	StackName   string    `json:"stack_name"`
	Reason      string    `json:"reason"`
	DetectedAt  time.Time `json:"detected_at"`
	Cleaned     bool      `json:"cleaned"`
}

// ServiceInfo is the runtime view of a Docker service.
type ServiceInfo struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Stack          string        `json:"stack"`
	PublishedPorts []string      `json:"published_ports"`
	Violation      ViolationInfo `json:"violation"`
}

// ViolationInfo marks whether a service violates policy.
type ViolationInfo struct {
	IsViolation bool   `json:"is_violation"`
	Reason      string `json:"reason"`
}

// DashboardStats summarizes system state for the UI.
type DashboardStats struct {
	StackCount       int  `json:"stack_count"`
	ServiceCount     int  `json:"service_count"`
	ViolationCount   int  `json:"violation_count"`
	AutoCleanEnabled bool `json:"auto_clean_enabled"`
}

// APIResponse is a standard JSON response envelope.
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// CreateStackRequest is the body for creating a stack.
type CreateStackRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateStackRequest is the body for updating a stack.
type UpdateStackRequest struct {
	Description string `json:"description"`
}

// AddPortRequest is the body for adding a stack port.
type AddPortRequest struct {
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
}

// ViolationStack groups violating services under a stack name.
type ViolationStack struct {
	Name         string   `json:"name"`
	ServiceCount int      `json:"service_count"`
	Services     []string `json:"services"`
	Ports        []string `json:"ports"`
	Reasons      []string `json:"reasons"`
	Configured   bool     `json:"configured"`
}

// WhitelistStackRequest one-click create stack + ports.
type WhitelistStackRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// WhitelistStackResult is the result of whitelist action.
type WhitelistStackResult struct {
	Stack           *Stack   `json:"stack"`
	Created         bool     `json:"created"`
	AddedPorts      []string `json:"added_ports"`
	SkippedPorts    []string `json:"skipped_ports"`
	MatchedServices []string `json:"matched_services"`
}