package persistence

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/core"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	FsyncPolicyAlways   = "always"
	FsyncPolicyInterval = "interval"
	FsyncPolicyOff      = "off"

	walOpPut    = "put"
	walOpDelete = "delete"
)

// DurabilityConfig defines persistence durability controls.
type DurabilityConfig struct {
	WALEnabled                 bool
	FsyncPolicy                string
	FsyncInterval              time.Duration
	ChecksumValidationInterval time.Duration
	StartupRepair              bool
}

// DefaultDurabilityConfig returns the default durability profile.
func DefaultDurabilityConfig() DurabilityConfig {
	return DurabilityConfig{
		WALEnabled:                 true,
		FsyncPolicy:                FsyncPolicyInterval,
		FsyncInterval:              1 * time.Second,
		ChecksumValidationInterval: 0,
		StartupRepair:              true,
	}
}

func (c DurabilityConfig) normalized() DurabilityConfig {
	n := c
	n.FsyncPolicy = strings.ToLower(strings.TrimSpace(n.FsyncPolicy))
	if n.FsyncPolicy == "" {
		n.FsyncPolicy = FsyncPolicyInterval
	}
	if n.FsyncPolicy != FsyncPolicyAlways && n.FsyncPolicy != FsyncPolicyInterval && n.FsyncPolicy != FsyncPolicyOff {
		n.FsyncPolicy = FsyncPolicyInterval
	}
	if n.FsyncInterval <= 0 {
		n.FsyncInterval = 1 * time.Second
	}
	if n.ChecksumValidationInterval < 0 {
		n.ChecksumValidationInterval = 0
	}
	return n
}

type walRecord struct {
	Op      string       `msgpack:"op"`
	IndexID core.IndexID `msgpack:"index_id"`
	Data    []byte       `msgpack:"data,omitempty"`
}

type manifestEntry struct {
	Version    uint64 `json:"version"`
	Checkpoint string `json:"checkpoint"`
	CreatedAt  int64  `json:"created_at_unix"`
}

// IntegrityReport summarizes checksum validation results across persisted data files.
type IntegrityReport struct {
	CheckedFiles    int
	CorruptFiles    int
	RepairedEntries int
}

// Store handles file-based persistence of matrices
type Store struct {
	basePath string
	codec    *Codec

	durability DurabilityConfig
	walPath    string

	// In-memory index of persisted users
	index   map[core.IndexID]*Snapshot
	indexMu sync.RWMutex

	// Write coalescing
	pendingWrites map[core.IndexID]*core.Matrix
	writeMu       sync.Mutex
	flushInterval time.Duration
	walMu         sync.Mutex
	checkpointMu  sync.Mutex

	// Stats
	totalWrites uint64
	totalReads  uint64

	syncMu          sync.Mutex
	lastSync        time.Time
	manifestVersion uint64
}

// NewStore creates a new persistence store
func NewStore(basePath string, compress bool) (*Store, error) {
	return NewStoreWithDurability(basePath, compress, DefaultDurabilityConfig())
}

// NewStoreWithDurability creates a new persistence store with durability settings.
func NewStoreWithDurability(basePath string, compress bool, durability DurabilityConfig) (*Store, error) {
	durability = durability.normalized()

	// Create directories
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base path: %w", err)
	}

	dataPath := filepath.Join(basePath, "data")
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data path: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(basePath, "manifest"), 0755); err != nil {
		return nil, fmt.Errorf("failed to create manifest path: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(basePath, "checkpoints"), 0755); err != nil {
		return nil, fmt.Errorf("failed to create checkpoints path: %w", err)
	}

	s := &Store{
		basePath:      basePath,
		codec:         NewCodec(compress),
		durability:    durability,
		walPath:       filepath.Join(basePath, "wal.log"),
		index:         make(map[core.IndexID]*Snapshot),
		pendingWrites: make(map[core.IndexID]*core.Matrix),
		flushInterval: 1 * time.Second,
	}

	// Load index from disk
	if err := s.loadIndex(); err != nil {
		if !s.durability.StartupRepair {
			return nil, fmt.Errorf("failed to load index: %w", err)
		}
		if rebuildErr := s.rebuildIndex(); rebuildErr != nil {
			return nil, fmt.Errorf("failed to load index: %w (startup repair rebuild failed: %v)", err, rebuildErr)
		}
		if err := s.saveIndex(); err != nil {
			return nil, fmt.Errorf("failed to persist rebuilt index during startup repair: %w", err)
		}
	}

	applied, err := s.replayWAL()
	if err != nil {
		return nil, fmt.Errorf("failed to replay wal: %w", err)
	}
	if applied > 0 {
		if err := s.saveIndex(); err != nil {
			return nil, fmt.Errorf("failed to persist replayed index: %w", err)
		}
	}

	if s.durability.StartupRepair {
		if _, err := s.ValidateDataFiles(true); err != nil {
			return nil, fmt.Errorf("failed startup checksum validation/repair: %w", err)
		}
	}

	return s, nil
}

// Save persists a matrix to disk
func (s *Store) Save(matrix *core.Matrix) error {
	data, err := s.codec.Encode(matrix)
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	if err := s.appendWAL(walRecord{Op: walOpPut, IndexID: matrix.IndexID, Data: data}); err != nil {
		return err
	}

	s.writeMu.Lock()
	s.pendingWrites[matrix.IndexID] = matrix
	s.writeMu.Unlock()

	return s.flushUser(matrix.IndexID)
}

// SaveAsync queues a matrix for async persistence.
func (s *Store) SaveAsync(matrix *core.Matrix) error {
	data, err := s.codec.Encode(matrix)
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	if err := s.appendWAL(walRecord{Op: walOpPut, IndexID: matrix.IndexID, Data: data}); err != nil {
		return err
	}

	s.writeMu.Lock()
	s.pendingWrites[matrix.IndexID] = matrix
	s.writeMu.Unlock()

	return nil
}

// flushUser writes a specific user's matrix to disk
func (s *Store) flushUser(indexID core.IndexID) error {
	s.writeMu.Lock()
	matrix, ok := s.pendingWrites[indexID]
	if !ok {
		s.writeMu.Unlock()
		return nil
	}
	delete(s.pendingWrites, indexID)
	s.writeMu.Unlock()

	// Encode matrix
	data, err := s.codec.Encode(matrix)
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	filename := s.userFilePath(indexID)
	if err := s.writeAtomically(filename, data, 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	// Update index
	snapshot := CreateSnapshot(matrix)
	s.indexMu.Lock()
	s.index[indexID] = &snapshot
	s.totalWrites++
	s.indexMu.Unlock()

	// Save index
	return s.saveIndex()
}

// FlushAll writes all pending matrices
func (s *Store) FlushAll() error {
	s.writeMu.Lock()
	users := make([]core.IndexID, 0, len(s.pendingWrites))
	for id := range s.pendingWrites {
		users = append(users, id)
	}
	s.writeMu.Unlock()

	var lastErr error
	for _, id := range users {
		if err := s.flushUser(id); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Load retrieves a matrix from disk
func (s *Store) Load(indexID core.IndexID) (*core.Matrix, error) {
	filename := s.userFilePath(indexID)

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, core.ErrMatrixNotFound
		}
		return nil, fmt.Errorf("read failed: %w", err)
	}

	matrix, err := s.codec.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	s.indexMu.Lock()
	s.totalReads++
	s.indexMu.Unlock()

	return matrix, nil
}

// Exists checks if a user's matrix exists on disk
func (s *Store) Exists(indexID core.IndexID) bool {
	s.indexMu.RLock()
	_, ok := s.index[indexID]
	s.indexMu.RUnlock()

	if ok {
		return true
	}

	// Check file directly
	filename := s.userFilePath(indexID)
	_, err := os.Stat(filename)
	return err == nil
}

// Delete removes a user's matrix from disk
func (s *Store) Delete(indexID core.IndexID) error {
	if err := s.appendWAL(walRecord{Op: walOpDelete, IndexID: indexID}); err != nil {
		return err
	}

	filename := s.userFilePath(indexID)

	s.writeMu.Lock()
	delete(s.pendingWrites, indexID)
	s.writeMu.Unlock()

	s.indexMu.Lock()
	delete(s.index, indexID)
	s.indexMu.Unlock()

	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return err
	}

	return s.saveIndex()
}

// GetSnapshot returns the cached snapshot for a user
func (s *Store) GetSnapshot(indexID core.IndexID) (*Snapshot, bool) {
	s.indexMu.RLock()
	defer s.indexMu.RUnlock()

	snap, ok := s.index[indexID]
	return snap, ok
}

// ListIndexes returns all persisted user IDs
func (s *Store) ListIndexes() []core.IndexID {
	s.indexMu.RLock()
	defer s.indexMu.RUnlock()

	users := make([]core.IndexID, 0, len(s.index))
	for id := range s.index {
		users = append(users, id)
	}
	return users
}

// userFilePath returns the file path for a user's matrix
func (s *Store) userFilePath(indexID core.IndexID) string {
	return filepath.Join(s.basePath, "data", string(indexID)+".nrdb")
}

// loadIndex loads the index from disk
func (s *Store) loadIndex() error {
	err := s.loadIndexFromManifest()
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return s.rebuildIndex()
}

// saveIndex persists the index to disk
func (s *Store) saveIndex() error {
	s.checkpointMu.Lock()
	defer s.checkpointMu.Unlock()

	s.indexMu.RLock()
	snapshots := make([]Snapshot, 0, len(s.index))
	for _, snap := range s.index {
		snapshots = append(snapshots, *snap)
	}
	s.indexMu.RUnlock()

	data, err := msgpack.Marshal(snapshots)
	if err != nil {
		return err
	}

	syncVersion := s.manifestVersion + 1
	checkpointName := fmt.Sprintf("checkpoint-%020d.nrdb", syncVersion)
	checkpointRelPath := filepath.ToSlash(filepath.Join("checkpoints", checkpointName))
	checkpointPath := filepath.Join(s.basePath, "checkpoints", checkpointName)

	if err := s.writeAtomically(checkpointPath, data, 0644); err != nil {
		return err
	}

	manifest := manifestEntry{
		Version:    syncVersion,
		Checkpoint: checkpointRelPath,
		CreatedAt:  time.Now().Unix(),
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	manifestName := fmt.Sprintf("MANIFEST-%020d.json", syncVersion)
	manifestPath := filepath.Join(s.basePath, "manifest", manifestName)
	if err := s.writeAtomically(manifestPath, manifestData, 0644); err != nil {
		return err
	}

	currentPath := filepath.Join(s.basePath, "manifest", "CURRENT")
	if err := s.writeAtomically(currentPath, []byte(manifestName), 0644); err != nil {
		return err
	}

	legacyIndexPath := filepath.Join(s.basePath, "index.nrdb")
	if err := s.writeAtomically(legacyIndexPath, data, 0644); err != nil {
		return err
	}

	s.manifestVersion = syncVersion
	return nil
}

// ValidateDataFiles verifies checksums/decoding of persisted .nrdb files.
// When repair=true, corrupt files are removed and index entries are repaired.
func (s *Store) ValidateDataFiles(repair bool) (IntegrityReport, error) {
	report := IntegrityReport{}
	dataPath := filepath.Join(s.basePath, "data")

	entries, err := os.ReadDir(dataPath)
	if err != nil {
		return report, err
	}

	present := make(map[core.IndexID]struct{}, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".nrdb" {
			continue
		}

		report.CheckedFiles++
		indexID := core.IndexID(strings.TrimSuffix(entry.Name(), ".nrdb"))
		present[indexID] = struct{}{}
		path := filepath.Join(dataPath, entry.Name())

		raw, readErr := os.ReadFile(path)
		if readErr == nil {
			_, readErr = s.codec.Decode(raw)
		}

		if readErr == nil {
			continue
		}

		report.CorruptFiles++
		if !repair {
			continue
		}

		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return report, err
		}

		s.indexMu.Lock()
		delete(s.index, indexID)
		s.indexMu.Unlock()
		report.RepairedEntries++
		delete(present, indexID)
	}

	if repair {
		s.indexMu.Lock()
		for indexID := range s.index {
			if _, ok := present[indexID]; ok {
				continue
			}
			delete(s.index, indexID)
			report.RepairedEntries++
		}
		s.indexMu.Unlock()

		if report.RepairedEntries > 0 {
			if err := s.saveIndex(); err != nil {
				return report, err
			}
		}
	}

	return report, nil
}

// rebuildIndex rebuilds index from data files
func (s *Store) rebuildIndex() error {
	dataPath := filepath.Join(s.basePath, "data")

	entries, err := os.ReadDir(dataPath)
	if err != nil {
		return err
	}

	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	s.index = make(map[core.IndexID]*Snapshot)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".nrdb" {
			continue
		}

		indexID := core.IndexID(entry.Name()[:len(entry.Name())-5]) // Remove .nrdb

		// Load and create snapshot
		data, err := os.ReadFile(filepath.Join(dataPath, entry.Name()))
		if err != nil {
			continue
		}

		matrix, err := s.codec.Decode(data)
		if err != nil {
			continue
		}

		snap := CreateSnapshot(matrix)
		s.index[indexID] = &snap
	}

	return nil
}

func (s *Store) replayWAL() (int, error) {
	if !s.durability.WALEnabled {
		return 0, nil
	}

	s.walMu.Lock()
	defer s.walMu.Unlock()

	data, err := os.ReadFile(s.walPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	offset := 0
	applied := 0
	for {
		if len(data)-offset < 8 {
			break
		}

		recordLen := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		if recordLen <= 0 {
			break
		}
		if recordLen > len(data)-offset-8 {
			break
		}

		end := offset + 4 + recordLen + 4
		if end > len(data) {
			break
		}

		payload := data[offset+4 : offset+4+recordLen]
		checksum := binary.LittleEndian.Uint32(data[offset+4+recordLen : end])
		if crc32.ChecksumIEEE(payload) != checksum {
			break
		}

		var record walRecord
		if err := msgpack.Unmarshal(payload, &record); err != nil {
			break
		}

		if err := s.applyWALRecord(record); err != nil {
			return applied, err
		}

		offset = end
		applied++
	}

	if offset < len(data) {
		if err := s.truncateWALLocked(int64(offset)); err != nil {
			return applied, err
		}
	}

	return applied, nil
}

func (s *Store) applyWALRecord(record walRecord) error {
	switch record.Op {
	case walOpPut:
		if len(record.Data) == 0 {
			return nil
		}

		if err := s.writeAtomically(s.userFilePath(record.IndexID), record.Data, 0644); err != nil {
			return err
		}

		matrix, err := s.codec.Decode(record.Data)
		if err != nil {
			return err
		}

		snap := CreateSnapshot(matrix)
		s.indexMu.Lock()
		s.index[record.IndexID] = &snap
		s.indexMu.Unlock()

	case walOpDelete:
		if err := os.Remove(s.userFilePath(record.IndexID)); err != nil && !os.IsNotExist(err) {
			return err
		}

		s.indexMu.Lock()
		delete(s.index, record.IndexID)
		s.indexMu.Unlock()
	}

	return nil
}

func (s *Store) appendWAL(record walRecord) error {
	if !s.durability.WALEnabled {
		return nil
	}

	s.walMu.Lock()
	defer s.walMu.Unlock()

	payload, err := msgpack.Marshal(record)
	if err != nil {
		return err
	}

	buf := make([]byte, 4+len(payload)+4)
	binary.LittleEndian.PutUint32(buf[:4], uint32(len(payload)))
	copy(buf[4:4+len(payload)], payload)
	binary.LittleEndian.PutUint32(buf[4+len(payload):], crc32.ChecksumIEEE(payload))

	f, err := os.OpenFile(s.walPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(buf); err != nil {
		return err
	}

	if s.shouldSync() {
		if err := f.Sync(); err != nil {
			return err
		}
		if err := s.syncDir(filepath.Dir(s.walPath)); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) truncateWAL(size int64) error {
	if !s.durability.WALEnabled {
		return nil
	}

	s.walMu.Lock()
	defer s.walMu.Unlock()

	return s.truncateWALLocked(size)
}

func (s *Store) truncateWALLocked(size int64) error {

	f, err := os.OpenFile(s.walPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := f.Truncate(size); err != nil {
		return err
	}

	if s.shouldSync() {
		if err := f.Sync(); err != nil {
			return err
		}
		if err := s.syncDir(filepath.Dir(s.walPath)); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) loadIndexFromManifest() error {
	currentPath := filepath.Join(s.basePath, "manifest", "CURRENT")
	manifestName, err := os.ReadFile(currentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return os.ErrNotExist
		}
		return err
	}

	name := strings.TrimSpace(string(manifestName))
	if name == "" {
		return os.ErrNotExist
	}

	manifestPath := filepath.Join(s.basePath, "manifest", name)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	var manifest manifestEntry
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return err
	}

	checkpointPath := manifest.Checkpoint
	if !filepath.IsAbs(checkpointPath) {
		checkpointPath = filepath.Join(s.basePath, filepath.FromSlash(checkpointPath))
	}

	checkpointData, err := os.ReadFile(checkpointPath)
	if err != nil {
		return err
	}

	var snapshots []Snapshot
	if err := msgpack.Unmarshal(checkpointData, &snapshots); err != nil {
		return err
	}

	s.indexMu.Lock()
	s.index = make(map[core.IndexID]*Snapshot, len(snapshots))
	for i := range snapshots {
		snap := snapshots[i]
		s.index[snap.IndexID] = &snap
	}
	s.indexMu.Unlock()

	s.manifestVersion = manifest.Version
	return nil
}

func (s *Store) writeAtomically(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}

	syncNow := s.shouldSync()
	if syncNow {
		if err := f.Sync(); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return err
		}
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if syncNow {
		if err := s.syncDir(filepath.Dir(path)); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) shouldSync() bool {
	switch s.durability.FsyncPolicy {
	case FsyncPolicyOff:
		return false
	case FsyncPolicyAlways:
		return true
	default:
		now := time.Now()
		s.syncMu.Lock()
		defer s.syncMu.Unlock()
		if s.lastSync.IsZero() || now.Sub(s.lastSync) >= s.durability.FsyncInterval {
			s.lastSync = now
			return true
		}
		return false
	}
}

func (s *Store) syncDir(path string) error {
	if runtime.GOOS == "windows" {
		// Windows does not support fsync on directories in this mode.
		return nil
	}

	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Stats returns persistence statistics
func (s *Store) Stats() map[string]any {
	s.indexMu.RLock()
	defer s.indexMu.RUnlock()

	s.writeMu.Lock()
	pendingCount := len(s.pendingWrites)
	s.writeMu.Unlock()

	return map[string]any{
		"persisted_users": len(s.index),
		"pending_writes":  pendingCount,
		"total_writes":    s.totalWrites,
		"total_reads":     s.totalReads,
		"base_path":       s.basePath,
		"wal_enabled":     s.durability.WALEnabled,
		"fsync_policy":    s.durability.FsyncPolicy,
	}
}

// StartFlushWorker starts background flush worker
func (s *Store) StartFlushWorker(interval time.Duration) chan struct{} {
	stop := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				s.FlushAll()
				return
			case <-ticker.C:
				s.FlushAll()
			}
		}
	}()

	return stop
}

// StartChecksumValidationWorker starts periodic checksum validation over persisted data files.
func (s *Store) StartChecksumValidationWorker(interval time.Duration) chan struct{} {
	if interval <= 0 {
		return nil
	}

	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_, _ = s.ValidateDataFiles(false)
			}
		}
	}()

	return stop
}
