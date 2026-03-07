package origins

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StoreVersion is the current version of the store format.
const StoreVersion = 1

// Store provides persistence for origins.
type Store interface {
	// Load reads origins from storage.
	Load() ([]*Origin, error)
	// Save writes origins to storage.
	Save(origins []*Origin) error
}

// storeData is the JSON structure for persistence.
type storeData struct {
	Version int                `json:"version"`
	Origins []*persistedOrigin `json:"origins"`
}

// persistedOrigin is the JSON structure for a persisted origin.
// Note: Status is not persisted (runtime only).
type persistedOrigin struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Type      OriginType      `json:"type"`
	Config    json.RawMessage `json:"config"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
}

// JSONStore implements Store using a JSON file.
type JSONStore struct {
	filePath string
	mu       sync.RWMutex
}

// NewJSONStore creates a new JSON file store.
func NewJSONStore(filePath string) *JSONStore {
	return &JSONStore{
		filePath: filePath,
	}
}

// Load reads origins from the JSON file.
func (s *JSONStore) Load() ([]*Origin, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if file exists
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		// File doesn't exist, return empty list
		return []*Origin{}, nil
	}

	// Read file
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read origins file: %w", err)
	}

	// Handle empty file
	if len(data) == 0 {
		return []*Origin{}, nil
	}

	// Parse JSON
	var stored storeData
	if err := json.Unmarshal(data, &stored); err != nil {
		// File is corrupted, log error and return empty list
		log.Printf("WARN: origins file is corrupted, starting with empty list: %v", err)
		return []*Origin{}, nil
	}

	// Check version
	if stored.Version != StoreVersion {
		log.Printf("WARN: origins file version %d differs from current %d, attempting migration",
			stored.Version, StoreVersion)
		// For now, just load what we can
	}

	// Convert to Origins
	origins := make([]*Origin, 0, len(stored.Origins))
	for _, po := range stored.Origins {
		origin, err := s.persistedToOrigin(po)
		if err != nil {
			log.Printf("WARN: skipping invalid origin %s: %v", po.ID, err)
			continue
		}
		// Set default status (not persisted)
		origin.Status = StatusConfigured
		origins = append(origins, origin)
	}

	return origins, nil
}

// Save writes origins to the JSON file.
func (s *JSONStore) Save(origins []*Origin) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Convert to persisted format
	persisted := make([]*persistedOrigin, len(origins))
	for i, o := range origins {
		po, err := s.originToPersisted(o)
		if err != nil {
			return fmt.Errorf("failed to convert origin %s: %w", o.ID, err)
		}
		persisted[i] = po
	}

	// Create store data
	data := storeData{
		Version: StoreVersion,
		Origins: persisted,
	}

	// Marshal to JSON with indentation for readability
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal origins: %w", err)
	}

	// Write to temp file first, then rename (atomic write)
	tempFile := s.filePath + ".tmp"
	if err := os.WriteFile(tempFile, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write origins file: %w", err)
	}

	// Rename temp file to actual file (atomic on most systems)
	if err := os.Rename(tempFile, s.filePath); err != nil {
		// Try to clean up temp file
		_ = os.Remove(tempFile)
		return fmt.Errorf("failed to save origins file: %w", err)
	}

	return nil
}

// persistedToOrigin converts a persisted origin to an Origin.
func (s *JSONStore) persistedToOrigin(po *persistedOrigin) (*Origin, error) {
	if po.ID == "" {
		return nil, errors.New("origin ID is required")
	}

	origin := &Origin{
		ID:   po.ID,
		Name: po.Name,
		Type: po.Type,
	}

	// Parse timestamps
	if po.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, po.CreatedAt); err == nil {
			origin.CreatedAt = t
		}
	}
	if po.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, po.UpdatedAt); err == nil {
			origin.UpdatedAt = t
		}
	}

	// Parse config based on type
	if err := origin.Config.UnmarshalForType(po.Config, po.Type); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return origin, nil
}

// originToPersisted converts an Origin to persisted format.
func (s *JSONStore) originToPersisted(o *Origin) (*persistedOrigin, error) {
	configData, err := json.Marshal(o.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	return &persistedOrigin{
		ID:        o.ID,
		Name:      o.Name,
		Type:      o.Type,
		Config:    configData,
		CreatedAt: o.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: o.UpdatedAt.UTC().Format(time.RFC3339),
	}, nil
}
