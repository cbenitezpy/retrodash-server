package origins

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Manager manages origins with thread-safe operations.
type Manager struct {
	store          Store
	origins        map[string]*Origin
	activeOriginID string
	mu             sync.RWMutex
}

// NewManager creates a new Manager with the given store.
func NewManager(store Store) *Manager {
	return &Manager{
		store:   store,
		origins: make(map[string]*Origin),
	}
}

// Load loads origins from the store.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	origins, err := m.store.Load()
	if err != nil {
		return fmt.Errorf("failed to load origins: %w", err)
	}

	m.origins = make(map[string]*Origin, len(origins))
	for _, o := range origins {
		m.origins[o.ID] = o
	}

	return nil
}

// save persists the current origins to the store.
// Caller must hold the lock.
func (m *Manager) save() error {
	origins := make([]*Origin, 0, len(m.origins))
	for _, o := range m.origins {
		origins = append(origins, o)
	}
	return m.store.Save(origins)
}

// GetAll returns all origins.
func (m *Manager) GetAll() []*Origin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Origin, 0, len(m.origins))
	for _, o := range m.origins {
		result = append(result, o)
	}
	return result
}

// GetByID returns an origin by ID.
func (m *Manager) GetByID(id string) (*Origin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	origin, exists := m.origins[id]
	if !exists {
		return nil, errors.New("origin not found")
	}
	return origin, nil
}

// Create creates a new origin.
func (m *Manager) Create(name string, originType OriginType, config OriginConfig) (*Origin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate name uniqueness (case-insensitive)
	if m.nameExists(name, "") {
		return nil, errors.New("origin with this name already exists")
	}

	// Create origin
	origin := NewOrigin(name, originType, config)

	// Validate
	if err := origin.Validate(); err != nil {
		return nil, err
	}

	// Add to map
	m.origins[origin.ID] = origin

	// Persist
	if err := m.save(); err != nil {
		// Rollback
		delete(m.origins, origin.ID)
		return nil, fmt.Errorf("failed to save origin: %w", err)
	}

	return origin, nil
}

// Update updates an existing origin.
func (m *Manager) Update(id string, name *string, config *OriginConfig) (*Origin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	origin, exists := m.origins[id]
	if !exists {
		return nil, errors.New("origin not found")
	}

	// Create a copy for update
	updated := *origin

	// Update name if provided
	if name != nil {
		// Validate name uniqueness (case-insensitive, excluding current origin)
		if m.nameExists(*name, id) {
			return nil, errors.New("origin with this name already exists")
		}
		updated.Name = *name
	}

	// Update config if provided
	if config != nil {
		// Validate config matches type
		if origin.Type == OriginTypeGrafana && config.Grafana == nil {
			return nil, errors.New("grafana type requires grafana config")
		}
		if origin.Type == OriginTypeCommand && config.Command == nil {
			return nil, errors.New("command type requires command config")
		}
		updated.Config = *config
	}

	// Update timestamp
	updated.UpdatedAt = time.Now().UTC()

	// Validate
	if err := updated.Validate(); err != nil {
		return nil, err
	}

	// Update in map
	m.origins[id] = &updated

	// Persist
	if err := m.save(); err != nil {
		// Rollback
		m.origins[id] = origin
		return nil, fmt.Errorf("failed to save origin: %w", err)
	}

	return &updated, nil
}

// Delete deletes an origin by ID.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	origin, exists := m.origins[id]
	if !exists {
		return errors.New("origin not found")
	}

	// If this origin is active, clear active origin
	if m.activeOriginID == id {
		m.activeOriginID = ""
	}

	// Delete from map
	delete(m.origins, id)

	// Persist
	if err := m.save(); err != nil {
		// Rollback
		m.origins[id] = origin
		return fmt.Errorf("failed to save after delete: %w", err)
	}

	return nil
}

// nameExists checks if an origin with the given name exists (case-insensitive).
// excludeID can be used to exclude an origin from the check (for updates).
func (m *Manager) nameExists(name string, excludeID string) bool {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	for id, o := range m.origins {
		if id != excludeID && strings.ToLower(strings.TrimSpace(o.Name)) == lowerName {
			return true
		}
	}
	return false
}

// GetActiveOriginID returns the ID of the currently active origin.
func (m *Manager) GetActiveOriginID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeOriginID
}

// SetActiveOriginID sets the active origin ID.
func (m *Manager) SetActiveOriginID(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id != "" {
		if _, exists := m.origins[id]; !exists {
			return errors.New("origin not found")
		}
	}
	m.activeOriginID = id
	return nil
}

// SetOriginStatus sets the status of an origin.
// This is a runtime-only operation (not persisted).
func (m *Manager) SetOriginStatus(id string, status ConnectionStatus, errorMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	origin, exists := m.origins[id]
	if !exists {
		return errors.New("origin not found")
	}

	origin.Status = status
	origin.ErrorMessage = errorMessage
	return nil
}

// GetOriginList returns the OriginList response.
func (m *Manager) GetOriginList() *OriginList {
	m.mu.RLock()
	defer m.mu.RUnlock()

	origins := make([]*Origin, 0, len(m.origins))
	for _, o := range m.origins {
		// Sanitize for response (remove API keys)
		origins = append(origins, o.SanitizeForResponse())
	}

	var activeID *string
	if m.activeOriginID != "" {
		activeID = &m.activeOriginID
	}

	return &OriginList{
		Origins:        origins,
		ActiveOriginID: activeID,
	}
}

// Count returns the number of origins.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.origins)
}
