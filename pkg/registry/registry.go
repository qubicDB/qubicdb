package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry represents a registered UUID with its metadata
type Entry struct {
	UUID      string         `json:"uuid"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// Store manages UUID registration with file-based persistence
type Store struct {
	entries  map[string]*Entry
	mu       sync.RWMutex
	filePath string
}

// NewStore creates a new registry store
func NewStore(dataPath string) (*Store, error) {
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create registry path: %w", err)
	}

	s := &Store{
		entries:  make(map[string]*Entry),
		filePath: filepath.Join(dataPath, "registry.json"),
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("failed to load registry: %w", err)
	}

	return s, nil
}

// Create registers a new UUID. Returns error if duplicate.
func (s *Store) Create(uuid string, metadata map[string]any) (*Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[uuid]; exists {
		return nil, fmt.Errorf("uuid already exists: %s", uuid)
	}

	now := time.Now()
	entry := &Entry{
		UUID:      uuid,
		Metadata:  metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.entries[uuid] = entry

	if err := s.save(); err != nil {
		delete(s.entries, uuid)
		return nil, fmt.Errorf("failed to persist: %w", err)
	}

	return entry, nil
}

// Get returns a registered entry by UUID
func (s *Store) Get(uuid string) (*Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.entries[uuid]
	return entry, ok
}

// Exists checks if a UUID is registered
func (s *Store) Exists(uuid string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.entries[uuid]
	return ok
}

// List returns all registered entries
func (s *Store) List() []*Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		result = append(result, entry)
	}
	return result
}

// Update modifies a registered entry. UUID can change (must stay unique).
func (s *Store) Update(oldUUID string, newUUID string, metadata map[string]any) (*Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.entries[oldUUID]
	if !exists {
		return nil, fmt.Errorf("uuid not found: %s", oldUUID)
	}

	// If UUID is changing, check new UUID is unique
	if newUUID != oldUUID {
		if _, dup := s.entries[newUUID]; dup {
			return nil, fmt.Errorf("new uuid already exists: %s", newUUID)
		}
	}

	// Update entry
	entry.UUID = newUUID
	entry.Metadata = metadata
	entry.UpdatedAt = time.Now()

	// If UUID changed, re-key the map
	if newUUID != oldUUID {
		delete(s.entries, oldUUID)
	}
	s.entries[newUUID] = entry

	if err := s.save(); err != nil {
		// Rollback
		if newUUID != oldUUID {
			delete(s.entries, newUUID)
			entry.UUID = oldUUID
			s.entries[oldUUID] = entry
		}
		return nil, fmt.Errorf("failed to persist: %w", err)
	}

	return entry, nil
}

// Delete removes a registered entry
func (s *Store) Delete(uuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[uuid]; !exists {
		return fmt.Errorf("uuid not found: %s", uuid)
	}

	deleted := s.entries[uuid]
	delete(s.entries, uuid)

	if err := s.save(); err != nil {
		s.entries[uuid] = deleted
		return fmt.Errorf("failed to persist: %w", err)
	}

	return nil
}

// FindOrCreate returns existing entry or creates a new one
func (s *Store) FindOrCreate(uuid string, metadata map[string]any) (*Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.entries[uuid]; exists {
		return entry, false, nil // found, not created
	}

	now := time.Now()
	entry := &Entry{
		UUID:      uuid,
		Metadata:  metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.entries[uuid] = entry

	if err := s.save(); err != nil {
		delete(s.entries, uuid)
		return nil, false, fmt.Errorf("failed to persist: %w", err)
	}

	return entry, true, nil // created
}

// Count returns the number of registered entries
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// ── Persistence ──────────────────────────────────────────────

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file yet
		}
		return err
	}

	var entries []*Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	for _, entry := range entries {
		s.entries[entry.UUID] = entry
	}

	return nil
}

func (s *Store) save() error {
	entries := make([]*Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		entries = append(entries, entry)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, s.filePath)
}
