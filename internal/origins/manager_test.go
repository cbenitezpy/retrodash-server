package origins

import (
	"strings"
	"testing"
)

// mockStore is a simple in-memory store for testing.
type mockStore struct {
	origins []*Origin
	saveErr error
	loadErr error
}

func (s *mockStore) Load() ([]*Origin, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.origins, nil
}

func (s *mockStore) Save(origins []*Origin) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.origins = origins
	return nil
}

func TestManager_Load(t *testing.T) {
	store := &mockStore{
		origins: []*Origin{
			{
				ID:   "id-1",
				Name: "Test Origin",
				Type: OriginTypeCommand,
				Config: OriginConfig{
					Command: &CommandConfig{Command: "cmd://htop"},
				},
			},
		},
	}

	manager := NewManager(store)
	if err := manager.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if manager.Count() != 1 {
		t.Errorf("Count() = %d, want 1", manager.Count())
	}
}

func TestManager_Create(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	config := OriginConfig{
		Command: &CommandConfig{Command: "cmd://htop"},
	}

	origin, err := manager.Create("Test Origin", OriginTypeCommand, config)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if origin.ID == "" {
		t.Error("Create() should generate an ID")
	}
	if origin.Name != "Test Origin" {
		t.Errorf("Name = %q, want %q", origin.Name, "Test Origin")
	}
	if origin.Status != StatusConfigured {
		t.Errorf("Status = %q, want %q", origin.Status, StatusConfigured)
	}

	// Verify it was saved
	if len(store.origins) != 1 {
		t.Errorf("Store has %d origins, want 1", len(store.origins))
	}
}

func TestManager_CreateDuplicateName(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	config := OriginConfig{
		Command: &CommandConfig{Command: "cmd://htop"},
	}

	// Create first origin
	_, err := manager.Create("Test Origin", OriginTypeCommand, config)
	if err != nil {
		t.Fatalf("First Create() error = %v", err)
	}

	// Try to create with same name
	_, err = manager.Create("Test Origin", OriginTypeCommand, config)
	if err == nil {
		t.Error("Create() should error on duplicate name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Error should mention 'already exists', got: %v", err)
	}

	// Try with different case
	_, err = manager.Create("TEST ORIGIN", OriginTypeCommand, config)
	if err == nil {
		t.Error("Create() should error on case-insensitive duplicate name")
	}
}

func TestManager_GetByID(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	config := OriginConfig{
		Command: &CommandConfig{Command: "cmd://htop"},
	}

	created, _ := manager.Create("Test Origin", OriginTypeCommand, config)

	// Get by valid ID
	origin, err := manager.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if origin.Name != "Test Origin" {
		t.Errorf("Name = %q, want %q", origin.Name, "Test Origin")
	}

	// Get by invalid ID
	_, err = manager.GetByID("invalid-id")
	if err == nil {
		t.Error("GetByID() should error for invalid ID")
	}
}

func TestManager_Update(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	config := OriginConfig{
		Command: &CommandConfig{Command: "cmd://htop"},
	}

	created, _ := manager.Create("Test Origin", OriginTypeCommand, config)

	// Update name
	newName := "Updated Origin"
	updated, err := manager.Update(created.ID, &newName, nil)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "Updated Origin" {
		t.Errorf("Name = %q, want %q", updated.Name, "Updated Origin")
	}

	// Verify UpdatedAt changed
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Error("UpdatedAt should be after original")
	}
}

func TestManager_UpdateDuplicateName(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	config := OriginConfig{
		Command: &CommandConfig{Command: "cmd://htop"},
	}

	// Create two origins
	manager.Create("Origin 1", OriginTypeCommand, config)
	origin2, _ := manager.Create("Origin 2", OriginTypeCommand, config)

	// Try to update origin2 with origin1's name
	newName := "Origin 1"
	_, err := manager.Update(origin2.ID, &newName, nil)
	if err == nil {
		t.Error("Update() should error on duplicate name")
	}
}

func TestManager_UpdateNotFound(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	newName := "Updated"
	_, err := manager.Update("invalid-id", &newName, nil)
	if err == nil {
		t.Error("Update() should error for non-existent origin")
	}
}

func TestManager_Delete(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	config := OriginConfig{
		Command: &CommandConfig{Command: "cmd://htop"},
	}

	created, _ := manager.Create("Test Origin", OriginTypeCommand, config)

	// Delete
	if err := manager.Delete(created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	if manager.Count() != 0 {
		t.Errorf("Count() = %d, want 0", manager.Count())
	}

	// Verify store was updated
	if len(store.origins) != 0 {
		t.Errorf("Store has %d origins, want 0", len(store.origins))
	}
}

func TestManager_DeleteNotFound(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	err := manager.Delete("invalid-id")
	if err == nil {
		t.Error("Delete() should error for non-existent origin")
	}
}

func TestManager_DeleteActiveOrigin(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	config := OriginConfig{
		Command: &CommandConfig{Command: "cmd://htop"},
	}

	created, _ := manager.Create("Test Origin", OriginTypeCommand, config)
	manager.SetActiveOriginID(created.ID)

	// Verify it's active
	if manager.GetActiveOriginID() != created.ID {
		t.Error("Origin should be active")
	}

	// Delete
	if err := manager.Delete(created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Active origin should be cleared
	if manager.GetActiveOriginID() != "" {
		t.Error("Active origin should be cleared after delete")
	}
}

func TestManager_SetOriginStatus(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	config := OriginConfig{
		Command: &CommandConfig{Command: "cmd://htop"},
	}

	created, _ := manager.Create("Test Origin", OriginTypeCommand, config)

	// Set status
	err := manager.SetOriginStatus(created.ID, StatusConnected, "")
	if err != nil {
		t.Fatalf("SetOriginStatus() error = %v", err)
	}

	// Verify status
	origin, _ := manager.GetByID(created.ID)
	if origin.Status != StatusConnected {
		t.Errorf("Status = %q, want %q", origin.Status, StatusConnected)
	}

	// Set error status
	err = manager.SetOriginStatus(created.ID, StatusError, "Connection failed")
	if err != nil {
		t.Fatalf("SetOriginStatus() error = %v", err)
	}

	origin, _ = manager.GetByID(created.ID)
	if origin.Status != StatusError {
		t.Errorf("Status = %q, want %q", origin.Status, StatusError)
	}
	if origin.ErrorMessage != "Connection failed" {
		t.Errorf("ErrorMessage = %q, want %q", origin.ErrorMessage, "Connection failed")
	}
}

func TestManager_GetOriginList(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	// Empty list
	list := manager.GetOriginList()
	if len(list.Origins) != 0 {
		t.Errorf("Origins length = %d, want 0", len(list.Origins))
	}
	if list.ActiveOriginID != nil {
		t.Error("ActiveOriginID should be nil")
	}

	// Add origins
	config1 := OriginConfig{
		Grafana: &GrafanaConfig{
			URL:    "https://grafana.example.com",
			APIKey: "secret-key",
		},
	}
	created, _ := manager.Create("Grafana", OriginTypeGrafana, config1)
	manager.SetActiveOriginID(created.ID)

	list = manager.GetOriginList()
	if len(list.Origins) != 1 {
		t.Fatalf("Origins length = %d, want 1", len(list.Origins))
	}

	// API key should be sanitized
	if list.Origins[0].Config.Grafana.APIKey != "" {
		t.Error("API key should be sanitized in response")
	}

	// Active origin should be set
	if list.ActiveOriginID == nil || *list.ActiveOriginID != created.ID {
		t.Error("ActiveOriginID should be set")
	}
}

func TestManager_ErrorIsolation(t *testing.T) {
	// T023a: Test that origin errors don't affect other origins
	store := &mockStore{}
	manager := NewManager(store)

	config := OriginConfig{
		Command: &CommandConfig{Command: "cmd://htop"},
	}

	// Create two origins
	origin1, _ := manager.Create("Origin 1", OriginTypeCommand, config)
	origin2, _ := manager.Create("Origin 2", OriginTypeCommand, config)

	// Set origin1 to error state
	manager.SetOriginStatus(origin1.ID, StatusError, "Connection failed")

	// Origin2 should still be configurable and accessible
	o2, err := manager.GetByID(origin2.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if o2.Status != StatusConfigured {
		t.Errorf("Origin2 status = %q, want %q", o2.Status, StatusConfigured)
	}

	// Should be able to update origin2 even though origin1 has error
	newName := "Updated Origin 2"
	updated, err := manager.Update(origin2.ID, &newName, nil)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "Updated Origin 2" {
		t.Errorf("Name = %q, want %q", updated.Name, "Updated Origin 2")
	}

	// Origin1 should still be in error state
	o1, _ := manager.GetByID(origin1.ID)
	if o1.Status != StatusError {
		t.Errorf("Origin1 status = %q, want %q", o1.Status, StatusError)
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	store := &mockStore{}
	manager := NewManager(store)

	config := OriginConfig{
		Command: &CommandConfig{Command: "cmd://htop"},
	}

	// Create some initial origins
	for i := 0; i < 10; i++ {
		manager.Create("Origin "+string(rune('A'+i)), OriginTypeCommand, config)
	}

	// Run concurrent operations
	done := make(chan bool)

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			manager.GetAll()
			manager.GetOriginList()
		}
		done <- true
	}()

	// Concurrent status updates
	go func() {
		origins := manager.GetAll()
		for i := 0; i < 100; i++ {
			for _, o := range origins {
				manager.SetOriginStatus(o.ID, StatusConnecting, "")
			}
		}
		done <- true
	}()

	// Wait for completion
	<-done
	<-done

	// Should not panic or deadlock
	if manager.Count() != 10 {
		t.Errorf("Count() = %d, want 10", manager.Count())
	}
}
