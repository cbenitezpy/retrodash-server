package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/health"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/origins"
	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBrowserStatus implements health.StatusProvider
type mockBrowserStatus struct {
	ready     bool
	lastError error
}

func (m *mockBrowserStatus) IsReady() bool {
	return m.ready
}

func (m *mockBrowserStatus) LastError() error {
	return m.lastError
}

// mockClientCounter implements health.ClientCounter
type mockClientCounter struct {
	count int
}

func (m *mockClientCounter) ActiveClients() int {
	return m.count
}

// mockModeProvider implements health.ModeProvider
type mockModeProvider struct {
	mode string
}

func (m *mockModeProvider) Mode() string {
	if m.mode != "" {
		return m.mode
	}
	return "browser"
}

func TestHealthHandler_OK(t *testing.T) {
	browser := &mockBrowserStatus{ready: true}
	clients := &mockClientCounter{count: 2}
	modeProvider := &mockModeProvider{mode: "browser"}
	checker := health.NewChecker(browser, clients, modeProvider)
	handlers := NewHandlers(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handlers.HealthHandler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp types.HealthResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.Equal(t, "ready", resp.BrowserStatus)
	assert.Equal(t, 2, resp.ActiveClients)
	assert.Greater(t, resp.Uptime, int64(-1))
}

func TestHealthHandler_Error(t *testing.T) {
	browser := &mockBrowserStatus{
		ready:     false,
		lastError: ErrBrowserNotReady,
	}
	modeProvider := &mockModeProvider{mode: "browser"}
	checker := health.NewChecker(browser, nil, modeProvider)
	handlers := NewHandlers(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handlers.HealthHandler(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp types.HealthResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, "error", resp.Status)
	assert.NotEmpty(t, resp.LastError)
}

func TestHealthHandler_MethodNotAllowed(t *testing.T) {
	modeProvider := &mockModeProvider{mode: "browser"}
	checker := health.NewChecker(nil, nil, modeProvider)
	handlers := NewHandlers(checker)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()

	handlers.HealthHandler(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHealthHandler_NoBrowser(t *testing.T) {
	// No browser configured yet (startup)
	modeProvider := &mockModeProvider{mode: "browser"}
	checker := health.NewChecker(nil, nil, modeProvider)
	handlers := NewHandlers(checker)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handlers.HealthHandler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp types.HealthResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "1.0.0", resp.Version)
}

// Placeholder error for testing
var ErrBrowserNotReady = assert.AnError

// mockOriginStore implements origins.Store interface for testing.
type mockOriginStore struct {
	origins []*origins.Origin
	saveErr error
	loadErr error
}

func (s *mockOriginStore) Load() ([]*origins.Origin, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.origins, nil
}

func (s *mockOriginStore) Save(o []*origins.Origin) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.origins = o
	return nil
}

// createTestOriginManager creates a manager with test data.
func createTestOriginManager() *origins.Manager {
	store := &mockOriginStore{}
	manager := origins.NewManager(store)
	return manager
}

// --- GET /api/origins Tests (T028) ---

func TestListOriginsHandler_EmptyList(t *testing.T) {
	manager := createTestOriginManager()
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	req := httptest.NewRequest(http.MethodGet, "/api/origins", nil)
	rec := httptest.NewRecorder()

	handlers.ListOriginsHandler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp origins.OriginList
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Empty(t, resp.Origins)
	assert.Nil(t, resp.ActiveOriginID)
}

func TestListOriginsHandler_WithOrigins(t *testing.T) {
	manager := createTestOriginManager()

	// Create test origins
	config1 := origins.OriginConfig{
		Grafana: &origins.GrafanaConfig{
			URL:    "https://grafana.example.com/d/abc123",
			APIKey: "secret-key-should-be-hidden",
		},
	}
	origin1, err := manager.Create("Production Grafana", origins.OriginTypeGrafana, config1)
	require.NoError(t, err)

	config2 := origins.OriginConfig{
		Command: &origins.CommandConfig{Command: "cmd://htop"},
	}
	_, err = manager.Create("System Monitor", origins.OriginTypeCommand, config2)
	require.NoError(t, err)

	// Set active origin
	require.NoError(t, manager.SetActiveOriginID(origin1.ID))

	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	req := httptest.NewRequest(http.MethodGet, "/api/origins", nil)
	rec := httptest.NewRecorder()

	handlers.ListOriginsHandler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp origins.OriginList
	err = json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Len(t, resp.Origins, 2)
	assert.NotNil(t, resp.ActiveOriginID)
	assert.Equal(t, origin1.ID, *resp.ActiveOriginID)

	// Verify API key is sanitized
	for _, o := range resp.Origins {
		if o.Config.Grafana != nil {
			assert.Empty(t, o.Config.Grafana.APIKey, "API key should be sanitized")
		}
	}
}

func TestListOriginsHandler_MethodNotAllowed(t *testing.T) {
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(createTestOriginManager())

	req := httptest.NewRequest(http.MethodPost, "/api/origins", nil)
	rec := httptest.NewRecorder()

	handlers.ListOriginsHandler(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestListOriginsHandler_NoManager(t *testing.T) {
	handlers := NewHandlers(nil)
	// originManager not set

	req := httptest.NewRequest(http.MethodGet, "/api/origins", nil)
	rec := httptest.NewRecorder()

	handlers.ListOriginsHandler(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "ORIGINS_UNAVAILABLE", resp.Code)
}

// --- POST /api/origins Tests (T044) ---

func TestCreateOriginHandler_GrafanaOrigin(t *testing.T) {
	manager := createTestOriginManager()
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	body := `{
		"name": "Production Grafana",
		"type": "grafana",
		"config": {
			"url": "https://grafana.example.com/d/abc123",
			"apiKey": "secret-key"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/origins", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.CreateOriginHandler(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp origins.Origin
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "Production Grafana", resp.Name)
	assert.Equal(t, origins.OriginTypeGrafana, resp.Type)
	assert.Equal(t, origins.StatusConfigured, resp.Status)
	assert.NotNil(t, resp.Config.Grafana)
	assert.Equal(t, "https://grafana.example.com/d/abc123", resp.Config.Grafana.URL)
}

func TestCreateOriginHandler_CommandOrigin(t *testing.T) {
	manager := createTestOriginManager()
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	body := `{
		"name": "System Monitor",
		"type": "command",
		"config": {
			"command": "cmd://htop"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/origins", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.CreateOriginHandler(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp origins.Origin
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, "System Monitor", resp.Name)
	assert.Equal(t, origins.OriginTypeCommand, resp.Type)
	assert.NotNil(t, resp.Config.Command)
	assert.Equal(t, "cmd://htop", resp.Config.Command.Command)
}

func TestCreateOriginHandler_DuplicateName(t *testing.T) {
	manager := createTestOriginManager()

	// Create first origin
	config := origins.OriginConfig{
		Command: &origins.CommandConfig{Command: "cmd://htop"},
	}
	_, err := manager.Create("Existing Origin", origins.OriginTypeCommand, config)
	require.NoError(t, err)

	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	body := `{
		"name": "Existing Origin",
		"type": "command",
		"config": {
			"command": "cmd://top"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/origins", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.CreateOriginHandler(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var resp types.ErrorResponse
	err = json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "DUPLICATE_NAME", resp.Code)
}

func TestCreateOriginHandler_InvalidCommand(t *testing.T) {
	manager := createTestOriginManager()
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	body := `{
		"name": "Bad Command",
		"type": "command",
		"config": {
			"command": "cmd://rm"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/origins", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.CreateOriginHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "COMMAND_NOT_ALLOWED", resp.Code)
}

func TestCreateOriginHandler_InvalidGrafanaURL(t *testing.T) {
	manager := createTestOriginManager()
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	body := `{
		"name": "Bad URL",
		"type": "grafana",
		"config": {
			"url": "http://localhost:3000/dashboard"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/origins", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.CreateOriginHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "URL_BLOCKED", resp.Code)
}

func TestCreateOriginHandler_InvalidJSON(t *testing.T) {
	manager := createTestOriginManager()
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	body := `{ invalid json }`

	req := httptest.NewRequest(http.MethodPost, "/api/origins", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.CreateOriginHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "INVALID_REQUEST", resp.Code)
}

func TestCreateOriginHandler_MissingName(t *testing.T) {
	manager := createTestOriginManager()
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	body := `{
		"type": "command",
		"config": {
			"command": "cmd://htop"
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/origins", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.CreateOriginHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "INVALID_REQUEST", resp.Code)
}

func TestCreateOriginHandler_MethodNotAllowed(t *testing.T) {
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(createTestOriginManager())

	req := httptest.NewRequest(http.MethodGet, "/api/origins", nil)
	rec := httptest.NewRecorder()

	handlers.CreateOriginHandler(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// --- PUT /api/origins/{id} Tests (T062) ---

func TestUpdateOriginHandler_Success(t *testing.T) {
	manager := createTestOriginManager()

	// Create an origin first
	config := origins.OriginConfig{
		Grafana: &origins.GrafanaConfig{
			URL:    "https://grafana.example.com/d/abc123",
			APIKey: "old-key",
		},
	}
	origin, err := manager.Create("Original Name", origins.OriginTypeGrafana, config)
	require.NoError(t, err)

	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	body := `{
		"name": "Updated Name",
		"config": {
			"url": "https://grafana.example.com/d/xyz789",
			"apiKey": "new-key"
		}
	}`

	req := httptest.NewRequest(http.MethodPut, "/api/origins/"+origin.ID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.UpdateOriginHandler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp origins.Origin
	err = json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, origin.ID, resp.ID)
	assert.Equal(t, "Updated Name", resp.Name)
	assert.Equal(t, origins.OriginTypeGrafana, resp.Type)
	assert.NotNil(t, resp.Config.Grafana)
	assert.Equal(t, "https://grafana.example.com/d/xyz789", resp.Config.Grafana.URL)
}

func TestUpdateOriginHandler_NotFound(t *testing.T) {
	manager := createTestOriginManager()
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	body := `{
		"name": "Updated Name",
		"config": {
			"url": "https://grafana.example.com/d/xyz789"
		}
	}`

	req := httptest.NewRequest(http.MethodPut, "/api/origins/nonexistent-id", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.UpdateOriginHandler(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "ORIGIN_NOT_FOUND", resp.Code)
}

func TestUpdateOriginHandler_DuplicateName(t *testing.T) {
	manager := createTestOriginManager()

	// Create two origins
	config := origins.OriginConfig{
		Command: &origins.CommandConfig{Command: "cmd://htop"},
	}
	_, err := manager.Create("First Origin", origins.OriginTypeCommand, config)
	require.NoError(t, err)

	origin2, err := manager.Create("Second Origin", origins.OriginTypeCommand, config)
	require.NoError(t, err)

	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	// Try to rename second origin to first origin's name
	body := `{
		"name": "First Origin",
		"config": {
			"command": "cmd://htop"
		}
	}`

	req := httptest.NewRequest(http.MethodPut, "/api/origins/"+origin2.ID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.UpdateOriginHandler(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var resp types.ErrorResponse
	err = json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "DUPLICATE_NAME", resp.Code)
}

func TestUpdateOriginHandler_InvalidURL(t *testing.T) {
	manager := createTestOriginManager()

	// Create a grafana origin
	config := origins.OriginConfig{
		Grafana: &origins.GrafanaConfig{
			URL: "https://grafana.example.com/d/abc123",
		},
	}
	origin, err := manager.Create("Grafana Origin", origins.OriginTypeGrafana, config)
	require.NoError(t, err)

	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	// Try to update with blocked URL
	body := `{
		"name": "Grafana Origin",
		"config": {
			"url": "http://localhost:3000/dashboard"
		}
	}`

	req := httptest.NewRequest(http.MethodPut, "/api/origins/"+origin.ID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.UpdateOriginHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp types.ErrorResponse
	err = json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "URL_BLOCKED", resp.Code)
}

func TestUpdateOriginHandler_InvalidCommand(t *testing.T) {
	manager := createTestOriginManager()

	// Create a command origin
	config := origins.OriginConfig{
		Command: &origins.CommandConfig{Command: "cmd://htop"},
	}
	origin, err := manager.Create("Command Origin", origins.OriginTypeCommand, config)
	require.NoError(t, err)

	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	// Try to update with invalid command
	body := `{
		"name": "Command Origin",
		"config": {
			"command": "cmd://rm"
		}
	}`

	req := httptest.NewRequest(http.MethodPut, "/api/origins/"+origin.ID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.UpdateOriginHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp types.ErrorResponse
	err = json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "COMMAND_NOT_ALLOWED", resp.Code)
}

func TestUpdateOriginHandler_MissingID(t *testing.T) {
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(createTestOriginManager())

	body := `{"name": "Updated Name"}`

	req := httptest.NewRequest(http.MethodPut, "/api/origins/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.UpdateOriginHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "INVALID_REQUEST", resp.Code)
}

func TestUpdateOriginHandler_MethodNotAllowed(t *testing.T) {
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(createTestOriginManager())

	req := httptest.NewRequest(http.MethodGet, "/api/origins/some-id", nil)
	rec := httptest.NewRecorder()

	handlers.UpdateOriginHandler(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestUpdateOriginHandler_NoManager(t *testing.T) {
	handlers := NewHandlers(nil)
	// originManager not set

	body := `{"name": "Updated Name"}`
	req := httptest.NewRequest(http.MethodPut, "/api/origins/some-id", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.UpdateOriginHandler(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "ORIGINS_UNAVAILABLE", resp.Code)
}

// --- DELETE /api/origins/{id} Tests (T073) ---

func TestDeleteOriginHandler_Success(t *testing.T) {
	manager := createTestOriginManager()

	// Create an origin first
	config := origins.OriginConfig{
		Command: &origins.CommandConfig{Command: "cmd://htop"},
	}
	origin, err := manager.Create("Origin to Delete", origins.OriginTypeCommand, config)
	require.NoError(t, err)

	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	req := httptest.NewRequest(http.MethodDelete, "/api/origins/"+origin.ID, nil)
	rec := httptest.NewRecorder()

	handlers.DeleteOriginHandler(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())

	// Verify origin was deleted
	_, err = manager.GetByID(origin.ID)
	assert.Error(t, err)
}

func TestDeleteOriginHandler_NotFound(t *testing.T) {
	manager := createTestOriginManager()
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	req := httptest.NewRequest(http.MethodDelete, "/api/origins/nonexistent-id", nil)
	rec := httptest.NewRecorder()

	handlers.DeleteOriginHandler(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "ORIGIN_NOT_FOUND", resp.Code)
}

func TestDeleteOriginHandler_MissingID(t *testing.T) {
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(createTestOriginManager())

	req := httptest.NewRequest(http.MethodDelete, "/api/origins/", nil)
	rec := httptest.NewRecorder()

	handlers.DeleteOriginHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "INVALID_REQUEST", resp.Code)
}

func TestDeleteOriginHandler_MethodNotAllowed(t *testing.T) {
	handlers := NewHandlers(nil)
	handlers.SetOriginManager(createTestOriginManager())

	req := httptest.NewRequest(http.MethodPost, "/api/origins/some-id", nil)
	rec := httptest.NewRecorder()

	handlers.DeleteOriginHandler(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestDeleteOriginHandler_NoManager(t *testing.T) {
	handlers := NewHandlers(nil)
	// originManager not set

	req := httptest.NewRequest(http.MethodDelete, "/api/origins/some-id", nil)
	rec := httptest.NewRecorder()

	handlers.DeleteOriginHandler(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp types.ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "ORIGINS_UNAVAILABLE", resp.Code)
}

func TestDeleteOriginHandler_ActiveOrigin(t *testing.T) {
	manager := createTestOriginManager()

	// Create and set as active origin
	config := origins.OriginConfig{
		Command: &origins.CommandConfig{Command: "cmd://htop"},
	}
	origin, err := manager.Create("Active Origin", origins.OriginTypeCommand, config)
	require.NoError(t, err)
	require.NoError(t, manager.SetActiveOriginID(origin.ID))

	handlers := NewHandlers(nil)
	handlers.SetOriginManager(manager)

	req := httptest.NewRequest(http.MethodDelete, "/api/origins/"+origin.ID, nil)
	rec := httptest.NewRecorder()

	handlers.DeleteOriginHandler(rec, req)

	// Should still succeed but clear active origin
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify active origin was cleared
	list := manager.GetOriginList()
	assert.Nil(t, list.ActiveOriginID)
}
