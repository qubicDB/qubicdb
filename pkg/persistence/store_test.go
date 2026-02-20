package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qubicDB/qubicdb/pkg/core"
)

func setupTestStore(t *testing.T) (*Store, string) {
	return setupTestStoreWithDurability(t, DefaultDurabilityConfig())
}

func setupTestStoreWithDurability(t *testing.T, durability DurabilityConfig) (*Store, string) {
	tmpDir, err := os.MkdirTemp("", "qubicdb-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	store, err := NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	return store, tmpDir
}

func TestStoreCreation(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer os.RemoveAll(tmpDir)

	if store == nil {
		t.Fatal("NewStore returned nil")
	}
}

func TestStoreSaveAndLoad(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer os.RemoveAll(tmpDir)

	// Create matrix
	m := core.NewMatrix("user-1", core.DefaultBounds())
	n := core.NewNeuron("Test content", m.CurrentDim)
	m.Neurons[n.ID] = n

	// Save
	err := store.Save(m)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify via Exists
	if !store.Exists("user-1") {
		t.Error("User should exist after save")
	}

	// Load
	loaded, err := store.Load("user-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.IndexID != m.IndexID {
		t.Error("IndexID mismatch")
	}
	if len(loaded.Neurons) != 1 {
		t.Errorf("Expected 1 neuron, got %d", len(loaded.Neurons))
	}
}

func TestStoreExists(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer os.RemoveAll(tmpDir)

	// Non-existent user
	if store.Exists("nonexistent") {
		t.Error("Should not exist")
	}

	// Save and check
	m := core.NewMatrix("user-1", core.DefaultBounds())
	store.Save(m)

	if !store.Exists("user-1") {
		t.Error("Should exist after save")
	}
}

func TestStoreDelete(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer os.RemoveAll(tmpDir)

	m := core.NewMatrix("user-1", core.DefaultBounds())
	store.Save(m)

	if !store.Exists("user-1") {
		t.Fatal("Should exist before delete")
	}

	err := store.Delete("user-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if store.Exists("user-1") {
		t.Error("Should not exist after delete")
	}
}

func TestStoreLoadNonExistent(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer os.RemoveAll(tmpDir)

	_, err := store.Load("nonexistent")
	if err == nil {
		t.Error("Should fail for non-existent user")
	}
}

func TestStoreSaveAsync(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer os.RemoveAll(tmpDir)

	m := core.NewMatrix("user-1", core.DefaultBounds())

	if err := store.SaveAsync(m); err != nil {
		t.Fatalf("SaveAsync failed: %v", err)
	}

	// Flush to ensure async save completes
	err := store.FlushAll()
	if err != nil {
		t.Fatalf("FlushAll failed: %v", err)
	}

	if !store.Exists("user-1") {
		t.Error("Should exist after async save and flush")
	}
}

func TestStoreListIndexes(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer os.RemoveAll(tmpDir)

	// Save multiple users
	for _, id := range []string{"user-1", "user-2", "user-3"} {
		m := core.NewMatrix(core.IndexID(id), core.DefaultBounds())
		store.Save(m)
	}

	users := store.ListIndexes()

	if len(users) != 3 {
		t.Errorf("Expected 3 users, got %d", len(users))
	}
}

func TestStoreStats(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer os.RemoveAll(tmpDir)

	m := core.NewMatrix("user-1", core.DefaultBounds())
	store.Save(m)

	stats := store.Stats()

	if stats["persisted_users"].(int) != 1 {
		t.Errorf("Expected 1 persisted user in stats, got %v", stats["persisted_users"])
	}
	if stats["base_path"].(string) != tmpDir {
		t.Error("Base path mismatch in stats")
	}
}

func TestStoreCompression(t *testing.T) {
	// Test with compression
	tmpDir1, err := os.MkdirTemp("", "qubicdb-test-compress-*")
	if err != nil {
		t.Skip("Cannot create temp dir")
	}
	defer os.RemoveAll(tmpDir1)

	storeCompressed, err := NewStore(tmpDir1, true)
	if err != nil {
		t.Fatalf("Failed to create compressed store: %v", err)
	}

	// Test without compression
	tmpDir2, err := os.MkdirTemp("", "qubicdb-test-nocompress-*")
	if err != nil {
		t.Skip("Cannot create temp dir")
	}
	defer os.RemoveAll(tmpDir2)

	storeUncompressed, err := NewStore(tmpDir2, false)
	if err != nil {
		t.Fatalf("Failed to create uncompressed store: %v", err)
	}

	// Create matrix with content
	m1 := core.NewMatrix("user-1", core.DefaultBounds())
	for i := 0; i < 100; i++ {
		n := core.NewNeuron("Test content for compression testing with some longer text", m1.CurrentDim)
		m1.Neurons[n.ID] = n
	}

	m2 := core.NewMatrix("user-1", core.DefaultBounds())
	for i := 0; i < 100; i++ {
		n := core.NewNeuron("Test content for compression testing with some longer text", m2.CurrentDim)
		m2.Neurons[n.ID] = n
	}

	if err := storeCompressed.Save(m1); err != nil {
		t.Fatalf("Save compressed failed: %v", err)
	}
	if err := storeUncompressed.Save(m2); err != nil {
		t.Fatalf("Save uncompressed failed: %v", err)
	}

	// Both stores should have the user
	if !storeCompressed.Exists("user-1") {
		t.Error("Compressed store should have user")
	}
	if !storeUncompressed.Exists("user-1") {
		t.Error("Uncompressed store should have user")
	}

	t.Log("Compression test passed - both stores saved successfully")
}

func TestStoreConcurrentAccess(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer os.RemoveAll(tmpDir)

	// Concurrent saves
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			m := core.NewMatrix(core.IndexID("user-"+string(rune('A'+idx))), core.DefaultBounds())
			store.Save(m)
			done <- true
		}(i)
	}

	// Wait for all
	for i := 0; i < 10; i++ {
		<-done
	}

	users := store.ListIndexes()
	if len(users) != 10 {
		t.Errorf("Expected 10 users after concurrent saves, got %d", len(users))
	}
}

func TestStoreWALReplayFromAsyncWrite(t *testing.T) {
	durability := DurabilityConfig{
		WALEnabled:    true,
		FsyncPolicy:   FsyncPolicyOff,
		FsyncInterval: time.Second,
	}

	store, tmpDir := setupTestStoreWithDurability(t, durability)
	defer os.RemoveAll(tmpDir)

	m := core.NewMatrix("wal-user", core.DefaultBounds())
	n := core.NewNeuron("wal recovery content", m.CurrentDim)
	m.Neurons[n.ID] = n

	if err := store.SaveAsync(m); err != nil {
		t.Fatalf("SaveAsync failed: %v", err)
	}

	restarted, err := NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to restart store: %v", err)
	}

	if !restarted.Exists("wal-user") {
		t.Fatal("expected wal-user to be recovered from WAL")
	}

	loaded, err := restarted.Load("wal-user")
	if err != nil {
		t.Fatalf("expected recovered user to load successfully: %v", err)
	}
	if len(loaded.Neurons) != 1 {
		t.Fatalf("expected 1 recovered neuron, got %d", len(loaded.Neurons))
	}
}

func TestStoreWALReplayDeleteWins(t *testing.T) {
	durability := DurabilityConfig{
		WALEnabled:    true,
		FsyncPolicy:   FsyncPolicyOff,
		FsyncInterval: time.Second,
	}

	store, tmpDir := setupTestStoreWithDurability(t, durability)
	defer os.RemoveAll(tmpDir)

	m := core.NewMatrix("wal-delete-user", core.DefaultBounds())
	if err := store.SaveAsync(m); err != nil {
		t.Fatalf("SaveAsync failed: %v", err)
	}
	if err := store.Delete("wal-delete-user"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	restarted, err := NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to restart store: %v", err)
	}

	if restarted.Exists("wal-delete-user") {
		t.Fatal("expected deleted user to remain deleted after WAL replay")
	}
}

func TestStoreWALTruncationScan(t *testing.T) {
	durability := DurabilityConfig{
		WALEnabled:    true,
		FsyncPolicy:   FsyncPolicyOff,
		FsyncInterval: time.Second,
	}

	store, tmpDir := setupTestStoreWithDurability(t, durability)
	defer os.RemoveAll(tmpDir)

	m := core.NewMatrix("wal-tail-user", core.DefaultBounds())
	if err := store.SaveAsync(m); err != nil {
		t.Fatalf("SaveAsync failed: %v", err)
	}

	walPath := filepath.Join(tmpDir, "wal.log")
	before, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("failed to stat wal before corruption: %v", err)
	}

	f, err := os.OpenFile(walPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open wal for tail corruption: %v", err)
	}
	if _, err := f.Write([]byte{0x01, 0x02, 0x03}); err != nil {
		f.Close()
		t.Fatalf("failed to append trailing garbage: %v", err)
	}
	f.Close()

	restarted, err := NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to restart store after wal tail corruption: %v", err)
	}

	if !restarted.Exists("wal-tail-user") {
		t.Fatal("expected valid WAL prefix to be replayed")
	}

	after, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("failed to stat wal after truncation scan: %v", err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("expected WAL to truncate garbage tail, size before=%d after=%d", before.Size(), after.Size())
	}
}

func TestStoreWritesManifestCheckpoint(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer os.RemoveAll(tmpDir)

	m := core.NewMatrix("manifest-user", core.DefaultBounds())
	if err := store.Save(m); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	currentPath := filepath.Join(tmpDir, "manifest", "CURRENT")
	currentData, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("failed to read CURRENT manifest pointer: %v", err)
	}

	manifestName := strings.TrimSpace(string(currentData))
	if !strings.HasPrefix(manifestName, "MANIFEST-") {
		t.Fatalf("unexpected manifest filename: %q", manifestName)
	}

	manifestData, err := os.ReadFile(filepath.Join(tmpDir, "manifest", manifestName))
	if err != nil {
		t.Fatalf("failed to read manifest file %s: %v", manifestName, err)
	}

	var manifest manifestEntry
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("failed to parse manifest file: %v", err)
	}
	if manifest.Version == 0 {
		t.Fatal("expected manifest version to be set")
	}

	checkpointPath := filepath.Join(tmpDir, filepath.FromSlash(manifest.Checkpoint))
	if _, err := os.Stat(checkpointPath); err != nil {
		t.Fatalf("expected checkpoint file to exist at %s: %v", checkpointPath, err)
	}
}

func TestStoreValidateDataFilesDetectsCorruption(t *testing.T) {
	durability := DurabilityConfig{
		WALEnabled:    false,
		FsyncPolicy:   FsyncPolicyOff,
		FsyncInterval: time.Second,
	}

	store, tmpDir := setupTestStoreWithDurability(t, durability)
	defer os.RemoveAll(tmpDir)

	m := core.NewMatrix("corrupt-check-user", core.DefaultBounds())
	if err := store.Save(m); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	userPath := filepath.Join(tmpDir, "data", "corrupt-check-user.nrdb")
	if err := os.WriteFile(userPath, []byte("not-a-valid-nrdb"), 0644); err != nil {
		t.Fatalf("failed to corrupt user file: %v", err)
	}

	report, err := store.ValidateDataFiles(false)
	if err != nil {
		t.Fatalf("validate data files failed: %v", err)
	}
	if report.CheckedFiles != 1 {
		t.Fatalf("expected CheckedFiles=1, got %d", report.CheckedFiles)
	}
	if report.CorruptFiles != 1 {
		t.Fatalf("expected CorruptFiles=1, got %d", report.CorruptFiles)
	}
	if report.RepairedEntries != 0 {
		t.Fatalf("expected RepairedEntries=0 without repair, got %d", report.RepairedEntries)
	}

	if _, err := os.Stat(userPath); err != nil {
		t.Fatalf("expected corrupt file to remain when repair=false: %v", err)
	}
}

func TestStoreStartupRepairRemovesCorruptFiles(t *testing.T) {
	durability := DurabilityConfig{
		WALEnabled:    false,
		FsyncPolicy:   FsyncPolicyOff,
		FsyncInterval: time.Second,
		StartupRepair: true,
	}

	store, tmpDir := setupTestStoreWithDurability(t, durability)
	defer os.RemoveAll(tmpDir)

	m := core.NewMatrix("startup-repair-user", core.DefaultBounds())
	if err := store.Save(m); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	userPath := filepath.Join(tmpDir, "data", "startup-repair-user.nrdb")
	if err := os.WriteFile(userPath, []byte("broken-file"), 0644); err != nil {
		t.Fatalf("failed to corrupt user file: %v", err)
	}

	restarted, err := NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to restart store with startup repair: %v", err)
	}

	if restarted.Exists("startup-repair-user") {
		t.Fatal("expected corrupt user to be removed from index during startup repair")
	}
	if _, err := os.Stat(userPath); !os.IsNotExist(err) {
		t.Fatalf("expected corrupt file to be removed during startup repair, stat err=%v", err)
	}
}
