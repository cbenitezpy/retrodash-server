package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/browser"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/health"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/origins"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/stream"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/switching"
	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
)

// Handlers holds dependencies for HTTP handlers.
type Handlers struct {
	healthChecker  *health.Checker
	broadcaster    *stream.Broadcaster
	touchHandler   *browser.TouchHandler
	originManager  *origins.Manager
	sourceSwitcher *switching.SourceSwitcher
	clientIDGen    func() string
}

// NewHandlers creates new HTTP handlers.
func NewHandlers(healthChecker *health.Checker) *Handlers {
	return &Handlers{
		healthChecker: healthChecker,
		clientIDGen:   generateClientID,
	}
}

// SetBroadcaster sets the stream broadcaster.
func (h *Handlers) SetBroadcaster(b *stream.Broadcaster) {
	h.broadcaster = b
}

// SetTouchHandler sets the touch handler.
func (h *Handlers) SetTouchHandler(t *browser.TouchHandler) {
	h.touchHandler = t
}

// SetOriginManager sets the origin manager.
func (h *Handlers) SetOriginManager(m *origins.Manager) {
	h.originManager = m
}

// SetSourceSwitcher sets the source switcher for dynamic origin switching.
func (h *Handlers) SetSourceSwitcher(s *switching.SourceSwitcher) {
	h.sourceSwitcher = s
}

// OriginsHandler handles /api/origins requests (GET for list, POST for create).
func (h *Handlers) OriginsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListOriginsHandler(w, r)
	case http.MethodPost:
		h.CreateOriginHandler(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// AllowedCommandsResponse is the response for GET /api/origins/allowed-commands.
type AllowedCommandsResponse struct {
	Commands []string `json:"commands"`
}

// AllowedCommandsHandler handles GET /api/origins/allowed-commands requests.
func (h *Handlers) AllowedCommandsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	commands := origins.GetAllowedCommands()
	resp := AllowedCommandsResponse{Commands: commands}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, resp)
}

// OriginItemHandler handles /api/origins/{id} requests.
// Supports: GET (retrieve), PUT (update), DELETE (delete)
// Also handles:
// - /api/origins/{id}/connect (POST)
// - /api/origins/allowed-commands (GET)
//
// Note: We use suffix checking for sub-routes because Go's standard http.ServeMux
// doesn't support path parameters like {id}. This is a common pattern when not
// using a third-party router (chi, gorilla/mux, etc.).
func (h *Handlers) OriginItemHandler(w http.ResponseWriter, r *http.Request) {
	// Check for special endpoints (order matters: more specific paths first)
	if strings.HasSuffix(r.URL.Path, "/allowed-commands") {
		h.AllowedCommandsHandler(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/connect") {
		h.ConnectOriginHandler(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.GetOriginHandler(w, r)
	case http.MethodPut:
		h.UpdateOriginHandler(w, r)
	case http.MethodDelete:
		h.DeleteOriginHandler(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ListOriginsHandler handles GET /api/origins requests.
func (h *Handlers) ListOriginsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.originManager == nil {
		writeJSONError(w, "Origin manager not available", "ORIGINS_UNAVAILABLE", http.StatusServiceUnavailable)
		return
	}

	resp := h.originManager.GetOriginList()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, resp)
}

// GetOriginHandler handles GET /api/origins/{id} requests.
func (h *Handlers) GetOriginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.originManager == nil {
		writeJSONError(w, "Origin manager not available", "ORIGINS_UNAVAILABLE", http.StatusServiceUnavailable)
		return
	}

	// Extract origin ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/origins/")
	originID := strings.TrimSuffix(path, "/")
	if originID == "" {
		writeJSONError(w, "Origin ID is required", "INVALID_REQUEST", http.StatusBadRequest)
		return
	}

	origin, err := h.originManager.GetByID(originID)
	if err != nil {
		writeJSONError(w, "Origin not found", "ORIGIN_NOT_FOUND", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, origin)
}

// CreateOriginRequest is the request body for POST /api/origins.
type CreateOriginRequest struct {
	Name   string             `json:"name"`
	Type   origins.OriginType `json:"type"`
	Config struct {
		// Grafana config
		URL       string `json:"url,omitempty"`
		APIKey    string `json:"apiKey,omitempty"`
		VerifyTLS bool   `json:"verifyTls,omitempty"`
		// Command config
		Command string `json:"command,omitempty"`
	} `json:"config"`
}

// CreateOriginHandler handles POST /api/origins requests.
func (h *Handlers) CreateOriginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.originManager == nil {
		writeJSONError(w, "Origin manager not available", "ORIGINS_UNAVAILABLE", http.StatusServiceUnavailable)
		return
	}

	// Parse request body
	var req CreateOriginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request body", "INVALID_REQUEST", http.StatusBadRequest)
		return
	}

	// Validate name
	if req.Name == "" {
		writeJSONError(w, "Name is required", "INVALID_REQUEST", http.StatusBadRequest)
		return
	}

	// Validate type
	if req.Type != origins.OriginTypeGrafana && req.Type != origins.OriginTypeCommand {
		writeJSONError(w, "Type must be 'grafana' or 'command'", "INVALID_REQUEST", http.StatusBadRequest)
		return
	}

	// Build config based on type
	var config origins.OriginConfig
	switch req.Type {
	case origins.OriginTypeGrafana:
		if req.Config.URL == "" {
			writeJSONError(w, "URL is required for grafana type", "INVALID_REQUEST", http.StatusBadRequest)
			return
		}
		// Validate URL (SSRF protection)
		if err := origins.ValidateGrafanaURL(req.Config.URL); err != nil {
			writeJSONError(w, err.Error(), "URL_BLOCKED", http.StatusBadRequest)
			return
		}
		config = origins.OriginConfig{
			Grafana: &origins.GrafanaConfig{
				URL:       req.Config.URL,
				APIKey:    req.Config.APIKey,
				VerifyTLS: req.Config.VerifyTLS,
			},
		}
	case origins.OriginTypeCommand:
		if req.Config.Command == "" {
			writeJSONError(w, "Command is required for command type", "INVALID_REQUEST", http.StatusBadRequest)
			return
		}
		// Validate command (allowlist check)
		if err := origins.ValidateCommand(req.Config.Command); err != nil {
			writeJSONError(w, err.Error(), "COMMAND_NOT_ALLOWED", http.StatusBadRequest)
			return
		}
		config = origins.OriginConfig{
			Command: &origins.CommandConfig{
				Command: req.Config.Command,
			},
		}
	}

	// Create origin
	origin, err := h.originManager.Create(req.Name, req.Type, config)
	if err != nil {
		// Check for duplicate name
		if strings.Contains(err.Error(), "already exists") {
			writeJSONError(w, "An origin with this name already exists", "DUPLICATE_NAME", http.StatusConflict)
			return
		}
		writeJSONError(w, err.Error(), "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, origin)
}

// UpdateOriginRequest is the request body for PUT /api/origins/{id}.
type UpdateOriginRequest struct {
	Name   string `json:"name,omitempty"`
	Config struct {
		// Grafana config
		URL       string `json:"url,omitempty"`
		APIKey    string `json:"apiKey,omitempty"`
		VerifyTLS bool   `json:"verifyTls,omitempty"`
		// Command config
		Command string `json:"command,omitempty"`
	} `json:"config"`
}

// UpdateOriginHandler handles PUT /api/origins/{id} requests.
func (h *Handlers) UpdateOriginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.originManager == nil {
		writeJSONError(w, "Origin manager not available", "ORIGINS_UNAVAILABLE", http.StatusServiceUnavailable)
		return
	}

	// Extract origin ID from URL path
	// URL format: /api/origins/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/origins/")
	originID := strings.TrimSuffix(path, "/")
	if originID == "" {
		writeJSONError(w, "Origin ID is required", "INVALID_REQUEST", http.StatusBadRequest)
		return
	}

	// Get existing origin to determine type
	existingOrigin, err := h.originManager.GetByID(originID)
	if err != nil {
		writeJSONError(w, "Origin not found", "ORIGIN_NOT_FOUND", http.StatusNotFound)
		return
	}

	// Parse request body
	var req UpdateOriginRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		writeJSONError(w, "Invalid request body", "INVALID_REQUEST", http.StatusBadRequest)
		return
	}

	// Use existing name if not provided
	name := req.Name
	if name == "" {
		name = existingOrigin.Name
	}

	// Build config based on existing type (type cannot be changed)
	var config origins.OriginConfig
	switch existingOrigin.Type {
	case origins.OriginTypeGrafana:
		url := req.Config.URL
		if url == "" && existingOrigin.Config.Grafana != nil {
			url = existingOrigin.Config.Grafana.URL
		}
		if url != "" {
			// Validate URL (SSRF protection)
			if validateErr := origins.ValidateGrafanaURL(url); validateErr != nil {
				writeJSONError(w, validateErr.Error(), "URL_BLOCKED", http.StatusBadRequest)
				return
			}
		}
		apiKey := req.Config.APIKey
		if apiKey == "" && existingOrigin.Config.Grafana != nil {
			apiKey = existingOrigin.Config.Grafana.APIKey
		}
		config = origins.OriginConfig{
			Grafana: &origins.GrafanaConfig{
				URL:       url,
				APIKey:    apiKey,
				VerifyTLS: req.Config.VerifyTLS,
			},
		}
	case origins.OriginTypeCommand:
		command := req.Config.Command
		if command == "" && existingOrigin.Config.Command != nil {
			command = existingOrigin.Config.Command.Command
		}
		if command != "" {
			// Validate command (allowlist check)
			if validateErr := origins.ValidateCommand(command); validateErr != nil {
				writeJSONError(w, validateErr.Error(), "COMMAND_NOT_ALLOWED", http.StatusBadRequest)
				return
			}
		}
		config = origins.OriginConfig{
			Command: &origins.CommandConfig{
				Command: command,
			},
		}
	}

	// Update origin
	updatedOrigin, err := h.originManager.Update(originID, &name, &config)
	if err != nil {
		// Check for duplicate name
		if strings.Contains(err.Error(), "already exists") {
			writeJSONError(w, "An origin with this name already exists", "DUPLICATE_NAME", http.StatusConflict)
			return
		}
		// Check for not found
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, "Origin not found", "ORIGIN_NOT_FOUND", http.StatusNotFound)
			return
		}
		writeJSONError(w, err.Error(), "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, updatedOrigin)
}

// ConnectOriginHandler handles POST /api/origins/{id}/connect requests.
func (h *Handlers) ConnectOriginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.originManager == nil {
		writeJSONError(w, "Origin manager not available", "ORIGINS_UNAVAILABLE", http.StatusServiceUnavailable)
		return
	}

	// Extract origin ID from URL path: /api/origins/{id}/connect
	path := strings.TrimPrefix(r.URL.Path, "/api/origins/")
	path = strings.TrimSuffix(path, "/connect")
	originID := strings.TrimSuffix(path, "/")
	if originID == "" {
		writeJSONError(w, "Origin ID is required", "INVALID_REQUEST", http.StatusBadRequest)
		return
	}

	// Get origin to verify it exists
	origin, err := h.originManager.GetByID(originID)
	if err != nil {
		writeJSONError(w, "Origin not found", "ORIGIN_NOT_FOUND", http.StatusNotFound)
		return
	}

	// Switch to the new source if source switcher is available
	if h.sourceSwitcher != nil {
		ctx := r.Context()
		if err := h.sourceSwitcher.SwitchToOrigin(ctx, origin); err != nil {
			writeJSONError(w, fmt.Sprintf("Failed to switch source: %v", err), "SWITCH_FAILED", http.StatusInternalServerError)
			return
		}

		// Update touch handler from source switcher
		h.touchHandler = h.sourceSwitcher.GetTouchHandler()
	}

	// Set as active origin
	if err := h.originManager.SetActiveOriginID(originID); err != nil {
		writeJSONError(w, "Failed to set active origin", "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	// Return the origin with connection info
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, origin)
}

// DeleteOriginHandler handles DELETE /api/origins/{id} requests.
func (h *Handlers) DeleteOriginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.originManager == nil {
		writeJSONError(w, "Origin manager not available", "ORIGINS_UNAVAILABLE", http.StatusServiceUnavailable)
		return
	}

	// Extract origin ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/origins/")
	originID := strings.TrimSuffix(path, "/")
	if originID == "" {
		writeJSONError(w, "Origin ID is required", "INVALID_REQUEST", http.StatusBadRequest)
		return
	}

	// Check if origin exists
	_, err := h.originManager.GetByID(originID)
	if err != nil {
		writeJSONError(w, "Origin not found", "ORIGIN_NOT_FOUND", http.StatusNotFound)
		return
	}

	// Delete the origin (manager handles clearing activeOriginId if needed)
	if err := h.originManager.Delete(originID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, "Origin not found", "ORIGIN_NOT_FOUND", http.StatusNotFound)
			return
		}
		writeJSONError(w, err.Error(), "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	// Success - 204 No Content
	w.WriteHeader(http.StatusNoContent)
}

// HealthHandler handles GET /health requests.
func (h *Handlers) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := h.healthChecker.Check()

	w.Header().Set("Content-Type", "application/json")
	if resp.Status == "ok" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	writeJSON(w, resp)
}

// StreamHandler handles GET /stream requests for MJPEG streaming.
// Query parameters:
//   - quality: "high" (default) or "low"
//   - api_key: Grafana API key (reserved for future dynamic dashboard navigation)
//   - tz: IANA timezone e.g. "America/Argentina/Buenos_Aires" (reserved for future use)
func (h *Handlers) StreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.broadcaster == nil {
		http.Error(w, "Stream not available", http.StatusServiceUnavailable)
		return
	}

	// Parse quality parameter
	quality := types.QualityHigh
	if q := r.URL.Query().Get("quality"); q == "low" {
		quality = types.QualityLow
	}

	// Note: api_key query param is accepted but currently uses env var GRAFANA_API_KEY
	// Future: implement dynamic dashboard navigation with per-connection auth
	// _ = r.URL.Query().Get("api_key")

	// Create MJPEG writer
	writer, err := stream.NewMJPEGWriter(w)
	if err != nil {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Create client
	clientID := h.clientIDGen()
	client := stream.NewClient(clientID, quality, r.RemoteAddr, 2)

	// Subscribe to broadcaster
	h.broadcaster.Subscribe(client)
	defer h.broadcaster.Unsubscribe(clientID)

	// Write headers
	writer.WriteHeaders()

	// Stream until client disconnects
	if err := stream.StreamToClient(r.Context(), client, writer); err != nil &&
		!errors.Is(err, context.Canceled) {
		log.Printf("stream error for client %s: %v", clientID, err)
	}
}

// TouchHandler handles POST /touch requests for touch events.
func (h *Handlers) TouchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.touchHandler == nil {
		writeJSONError(w, "Touch handler not available", "TOUCH_UNAVAILABLE", http.StatusServiceUnavailable)
		return
	}

	// Parse touch event
	var event types.TouchEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeJSONError(w, "Invalid request body", "INVALID_REQUEST", http.StatusBadRequest)
		return
	}

	// Validate coordinates
	if !browser.ValidateTouchEvent(event) {
		writeJSONError(w, "Invalid coordinates: x and y must be between 0 and 1", "INVALID_TOUCH", http.StatusBadRequest)
		return
	}

	// Handle touch event
	if err := h.touchHandler.HandleTouch(r.Context(), event); err != nil {
		writeJSONError(w, err.Error(), "TOUCH_ERROR", http.StatusInternalServerError)
		return
	}

	// Success - 204 No Content
	w.WriteHeader(http.StatusNoContent)
}

// writeJSON encodes v as JSON into w, logging on failure.
func writeJSON(w http.ResponseWriter, v interface{}) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: failed to encode response: %v", err)
	}
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, message string, code string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	writeJSON(w, types.ErrorResponse{
		Message: message,
		Code:    code,
	})
}

// generateClientID generates a unique client ID.
func generateClientID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
