// Package origins provides origin data source management for RetroDash.
package origins

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// OriginType represents the type of origin data source.
type OriginType string

const (
	// OriginTypeGrafana represents a Grafana dashboard origin.
	OriginTypeGrafana OriginType = "grafana"
	// OriginTypeCommand represents a terminal command origin.
	OriginTypeCommand OriginType = "command"
)

// IsValid checks if the origin type is valid.
func (t OriginType) IsValid() bool {
	return t == OriginTypeGrafana || t == OriginTypeCommand
}

// ConnectionStatus represents the connection state of an origin.
type ConnectionStatus string

const (
	// StatusConfigured means the origin is saved but not connected.
	StatusConfigured ConnectionStatus = "configured"
	// StatusConnecting means a connection is being established.
	StatusConnecting ConnectionStatus = "connecting"
	// StatusConnected means the origin is actively connected.
	StatusConnected ConnectionStatus = "connected"
	// StatusError means the connection failed.
	StatusError ConnectionStatus = "error"
)

// GrafanaConfig holds configuration for a Grafana dashboard origin.
type GrafanaConfig struct {
	// URL is the Grafana dashboard URL (required).
	URL string `json:"url"`
	// APIKey is the optional Grafana API key (write-only, never returned in GET).
	APIKey string `json:"apiKey,omitempty"`
	// VerifyTLS enables TLS certificate verification (default: false for self-signed).
	VerifyTLS bool `json:"verifyTls"`
}

// Validate checks if the Grafana configuration is valid.
func (c *GrafanaConfig) Validate() error {
	if c.URL == "" {
		return errors.New("url is required")
	}
	if err := ValidateGrafanaURL(c.URL); err != nil {
		return err
	}
	return nil
}

// CommandConfig holds configuration for a terminal command origin.
type CommandConfig struct {
	// Command is the command to execute in cmd:// format.
	Command string `json:"command"`
}

// Validate checks if the command configuration is valid.
func (c *CommandConfig) Validate() error {
	if c.Command == "" {
		return errors.New("command is required")
	}
	if err := ValidateCommand(c.Command); err != nil {
		return err
	}
	return nil
}

// OriginConfig is a union type for origin configuration.
// It can be either GrafanaConfig or CommandConfig.
type OriginConfig struct {
	Grafana *GrafanaConfig `json:"-"`
	Command *CommandConfig `json:"-"`
}

// MarshalJSON implements json.Marshaler for OriginConfig.
func (c OriginConfig) MarshalJSON() ([]byte, error) {
	if c.Grafana != nil {
		return json.Marshal(c.Grafana)
	}
	if c.Command != nil {
		return json.Marshal(c.Command)
	}
	return []byte("null"), nil
}

// UnmarshalJSON implements json.Unmarshaler for OriginConfig.
// Note: This needs the origin type to properly unmarshal.
// Use UnmarshalForType instead when the type is known.
func (c *OriginConfig) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as GrafanaConfig first (has 'url' field)
	var grafana GrafanaConfig
	if err := json.Unmarshal(data, &grafana); err == nil && grafana.URL != "" {
		c.Grafana = &grafana
		return nil
	}

	// Try CommandConfig (has 'command' field)
	var command CommandConfig
	if err := json.Unmarshal(data, &command); err == nil && command.Command != "" {
		c.Command = &command
		return nil
	}

	return errors.New("invalid origin config: must have either 'url' or 'command' field")
}

// UnmarshalForType unmarshals config based on the origin type.
func (c *OriginConfig) UnmarshalForType(data []byte, originType OriginType) error {
	switch originType {
	case OriginTypeGrafana:
		var grafana GrafanaConfig
		if err := json.Unmarshal(data, &grafana); err != nil {
			return fmt.Errorf("invalid grafana config: %w", err)
		}
		c.Grafana = &grafana
	case OriginTypeCommand:
		var command CommandConfig
		if err := json.Unmarshal(data, &command); err != nil {
			return fmt.Errorf("invalid command config: %w", err)
		}
		c.Command = &command
	default:
		return fmt.Errorf("unknown origin type: %s", originType)
	}
	return nil
}

// Validate validates the config based on its type.
func (c *OriginConfig) Validate() error {
	if c.Grafana != nil {
		return c.Grafana.Validate()
	}
	if c.Command != nil {
		return c.Command.Validate()
	}
	return errors.New("config is empty")
}

// Origin represents a data source configuration.
type Origin struct {
	// ID is the unique identifier (UUID v4).
	ID string `json:"id"`
	// Name is the descriptive name (1-100 chars, unique case-insensitive).
	Name string `json:"name"`
	// Type is the origin type (grafana or command).
	Type OriginType `json:"type"`
	// Config is the type-specific configuration.
	Config OriginConfig `json:"config"`
	// Status is the runtime connection status (not persisted).
	Status ConnectionStatus `json:"status"`
	// ErrorMessage contains error details if status is 'error'.
	ErrorMessage string `json:"errorMessage,omitempty"`
	// CreatedAt is the creation timestamp (ISO 8601 UTC).
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt is the last modification timestamp (ISO 8601 UTC).
	UpdatedAt time.Time `json:"updatedAt"`
}

// NewOrigin creates a new Origin with generated ID and timestamps.
func NewOrigin(name string, originType OriginType, config OriginConfig) *Origin {
	now := time.Now().UTC()
	return &Origin{
		ID:        uuid.New().String(),
		Name:      name,
		Type:      originType,
		Config:    config,
		Status:    StatusConfigured,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Validate validates the origin data.
func (o *Origin) Validate() error {
	var errs []string

	// Validate name
	if err := ValidateName(o.Name); err != nil {
		errs = append(errs, err.Error())
	}

	// Validate type
	if !o.Type.IsValid() {
		errs = append(errs, fmt.Sprintf("invalid type: %s (must be 'grafana' or 'command')", o.Type))
	}

	// Validate config matches type
	if o.Type == OriginTypeGrafana && o.Config.Grafana == nil {
		errs = append(errs, "grafana type requires grafana config")
	}
	if o.Type == OriginTypeCommand && o.Config.Command == nil {
		errs = append(errs, "command type requires command config")
	}

	// Validate config
	if err := o.Config.Validate(); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// SanitizeForResponse removes sensitive fields from the origin for API responses.
func (o *Origin) SanitizeForResponse() *Origin {
	sanitized := *o
	if sanitized.Config.Grafana != nil {
		// Create a copy without the API key
		grafanaCopy := *sanitized.Config.Grafana
		grafanaCopy.APIKey = ""
		sanitized.Config.Grafana = &grafanaCopy
	}
	return &sanitized
}

// ValidateName validates an origin name.
func ValidateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("name is required")
	}
	if len(name) > 100 {
		return errors.New("name must be 100 characters or less")
	}
	return nil
}

// OriginList is the response for GET /api/origins.
type OriginList struct {
	// Origins is the list of all configured origins.
	Origins []*Origin `json:"origins"`
	// ActiveOriginID is the ID of the currently connected origin (null if none).
	ActiveOriginID *string `json:"activeOriginId"`
}

// CreateOriginRequest is the request body for POST /api/origins.
type CreateOriginRequest struct {
	Name   string          `json:"name"`
	Type   OriginType      `json:"type"`
	Config json.RawMessage `json:"config"`
}

// UpdateOriginRequest is the request body for PUT /api/origins/{id}.
type UpdateOriginRequest struct {
	Name   *string         `json:"name,omitempty"`
	Config json.RawMessage `json:"config,omitempty"`
}

// OriginStatus is the response for GET /api/origins/{id}/status.
type OriginStatus struct {
	ID           string           `json:"id"`
	Status       ConnectionStatus `json:"status"`
	ErrorMessage string           `json:"errorMessage,omitempty"`
}

// ErrorResponse is the standard error response.
type ErrorResponse struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// ValidationErrorResponse includes field-level validation errors.
type ValidationErrorResponse struct {
	Message string            `json:"message"`
	Code    string            `json:"code"`
	Errors  []ValidationError `json:"errors"`
}

// ValidationError represents a single field validation error.
type ValidationError struct {
	Field string `json:"field"`
	Error string `json:"error"`
}

// Error codes for programmatic handling.
const (
	ErrCodeInvalidRequest         = "INVALID_REQUEST"
	ErrCodeInvalidUUID            = "INVALID_UUID"
	ErrCodeOriginNotFound         = "ORIGIN_NOT_FOUND"
	ErrCodeDuplicateName          = "DUPLICATE_NAME"
	ErrCodeInvalidURL             = "INVALID_URL"
	ErrCodeInvalidCommand         = "INVALID_COMMAND"
	ErrCodeCommandNotAllowed      = "COMMAND_NOT_ALLOWED"
	ErrCodeURLBlocked             = "URL_BLOCKED"
	ErrCodeConnectionFailed       = "CONNECTION_FAILED"
	ErrCodeConnectionTimeout      = "CONNECTION_TIMEOUT"
	ErrCodeOriginAlreadyConnected = "ORIGIN_ALREADY_CONNECTED"
	ErrCodeOriginNotConnected     = "ORIGIN_NOT_CONNECTED"
	ErrCodeTypeChangeNotAllowed   = "TYPE_CHANGE_NOT_ALLOWED"
	ErrCodeMethodNotAllowed       = "METHOD_NOT_ALLOWED"
	ErrCodeInternalError          = "INTERNAL_ERROR"
)
