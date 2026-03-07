package origins

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJSONStore_LoadEmpty(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "origins_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewJSONStore(filepath.Join(tempDir, "origins.json"))

	// Load from non-existent file should return empty list
	origins, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(origins) != 0 {
		t.Errorf("Load() returned %d origins, want 0", len(origins))
	}
}

func TestJSONStore_SaveAndLoad(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "origins_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewJSONStore(filepath.Join(tempDir, "origins.json"))

	// Create test origins
	now := time.Now().UTC().Truncate(time.Second)
	origins := []*Origin{
		{
			ID:   "id-1",
			Name: "Grafana Dashboard",
			Type: OriginTypeGrafana,
			Config: OriginConfig{
				Grafana: &GrafanaConfig{
					URL:       "https://grafana.example.com/d/abc123",
					APIKey:    "secret-key",
					VerifyTLS: false,
				},
			},
			Status:    StatusConfigured,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:   "id-2",
			Name: "System Monitor",
			Type: OriginTypeCommand,
			Config: OriginConfig{
				Command: &CommandConfig{
					Command: "cmd://htop",
				},
			},
			Status:    StatusConfigured,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	// Save
	if saveErr := store.Save(origins); saveErr != nil {
		t.Fatalf("Save() error = %v", saveErr)
	}

	// Verify file exists
	if _, statErr := os.Stat(filepath.Join(tempDir, "origins.json")); os.IsNotExist(statErr) {
		t.Fatal("Save() did not create file")
	}

	// Load
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("Load() returned %d origins, want 2", len(loaded))
	}

	// Verify first origin
	if loaded[0].ID != "id-1" {
		t.Errorf("Origin[0].ID = %q, want %q", loaded[0].ID, "id-1")
	}
	if loaded[0].Name != "Grafana Dashboard" {
		t.Errorf("Origin[0].Name = %q, want %q", loaded[0].Name, "Grafana Dashboard")
	}
	if loaded[0].Type != OriginTypeGrafana {
		t.Errorf("Origin[0].Type = %q, want %q", loaded[0].Type, OriginTypeGrafana)
	}
	if loaded[0].Config.Grafana == nil {
		t.Fatal("Origin[0].Config.Grafana is nil")
	}
	if loaded[0].Config.Grafana.URL != "https://grafana.example.com/d/abc123" {
		t.Errorf("Origin[0].Config.Grafana.URL = %q, want %q",
			loaded[0].Config.Grafana.URL, "https://grafana.example.com/d/abc123")
	}
	// API key should be persisted
	if loaded[0].Config.Grafana.APIKey != "secret-key" {
		t.Errorf("Origin[0].Config.Grafana.APIKey = %q, want %q",
			loaded[0].Config.Grafana.APIKey, "secret-key")
	}

	// Verify second origin
	if loaded[1].ID != "id-2" {
		t.Errorf("Origin[1].ID = %q, want %q", loaded[1].ID, "id-2")
	}
	if loaded[1].Config.Command == nil {
		t.Fatal("Origin[1].Config.Command is nil")
	}
	if loaded[1].Config.Command.Command != "cmd://htop" {
		t.Errorf("Origin[1].Config.Command.Command = %q, want %q",
			loaded[1].Config.Command.Command, "cmd://htop")
	}

	// Verify status is set to configured on load
	if loaded[0].Status != StatusConfigured {
		t.Errorf("Origin[0].Status = %q, want %q", loaded[0].Status, StatusConfigured)
	}
}

func TestJSONStore_CorruptedFile(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "origins_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "origins.json")
	store := NewJSONStore(filePath)

	// Write corrupted JSON
	if writeErr := os.WriteFile(filePath, []byte("not valid json {{{"), 0644); writeErr != nil {
		t.Fatalf("Failed to write corrupted file: %v", writeErr)
	}

	// Load should return empty list (graceful handling)
	origins, err := store.Load()
	if err != nil {
		t.Fatalf("Load() should not error on corrupted file, got: %v", err)
	}
	if len(origins) != 0 {
		t.Errorf("Load() returned %d origins, want 0 for corrupted file", len(origins))
	}
}

func TestJSONStore_EmptyFile(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "origins_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "origins.json")
	store := NewJSONStore(filePath)

	// Write empty file
	if writeErr := os.WriteFile(filePath, []byte(""), 0644); writeErr != nil {
		t.Fatalf("Failed to write empty file: %v", writeErr)
	}

	// Load should return empty list
	origins, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(origins) != 0 {
		t.Errorf("Load() returned %d origins, want 0", len(origins))
	}
}

func TestJSONStore_CreateDirectoryIfNotExists(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "origins_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Use a nested path that doesn't exist
	store := NewJSONStore(filepath.Join(tempDir, "nested", "dir", "origins.json"))

	origins := []*Origin{
		{
			ID:   "id-1",
			Name: "Test",
			Type: OriginTypeCommand,
			Config: OriginConfig{
				Command: &CommandConfig{
					Command: "cmd://htop",
				},
			},
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}

	// Save should create the directory
	if err := store.Save(origins); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(filepath.Join(tempDir, "nested", "dir")); os.IsNotExist(err) {
		t.Error("Save() did not create nested directory")
	}
}

func TestJSONStore_AtomicWrite(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "origins_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "origins.json")
	store := NewJSONStore(filePath)

	origins := []*Origin{
		{
			ID:        "id-1",
			Name:      "Test",
			Type:      OriginTypeCommand,
			Config:    OriginConfig{Command: &CommandConfig{Command: "cmd://htop"}},
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}

	// Save
	if err := store.Save(origins); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Temp file should not exist after save
	if _, err := os.Stat(filePath + ".tmp"); !os.IsNotExist(err) {
		t.Error("Temp file should not exist after successful save")
	}
}

func TestJSONStore_InvalidOriginSkipped(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "origins_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "origins.json")
	store := NewJSONStore(filePath)

	// Write JSON with one valid and one invalid origin
	data := `{
		"version": 1,
		"origins": [
			{
				"id": "valid-id",
				"name": "Valid Origin",
				"type": "command",
				"config": {"command": "cmd://htop"},
				"createdAt": "2026-01-10T10:00:00Z",
				"updatedAt": "2026-01-10T10:00:00Z"
			},
			{
				"id": "",
				"name": "Invalid - no ID",
				"type": "command",
				"config": {"command": "cmd://htop"},
				"createdAt": "2026-01-10T10:00:00Z",
				"updatedAt": "2026-01-10T10:00:00Z"
			}
		]
	}`
	if writeErr := os.WriteFile(filePath, []byte(data), 0644); writeErr != nil {
		t.Fatalf("Failed to write file: %v", writeErr)
	}

	// Load should only return the valid origin
	origins, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(origins) != 1 {
		t.Errorf("Load() returned %d origins, want 1", len(origins))
	}
	if origins[0].ID != "valid-id" {
		t.Errorf("Origin ID = %q, want %q", origins[0].ID, "valid-id")
	}
}
