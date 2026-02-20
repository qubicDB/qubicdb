package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/persistence"
)

func TestE2EDurability_WALReplayAfterRestart(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qubicdb-e2e-durability-wal-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	durability := persistence.DurabilityConfig{
		WALEnabled:    true,
		FsyncPolicy:   persistence.FsyncPolicyOff,
		FsyncInterval: time.Second,
	}

	store1, err := persistence.NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to create first store: %v", err)
	}

	m := core.NewMatrix("e2e-wal-user", core.DefaultBounds())
	n := core.NewNeuron("e2e wal replay content", m.CurrentDim)
	m.Neurons[n.ID] = n
	if err := store1.SaveAsync(m); err != nil {
		t.Fatalf("SaveAsync failed: %v", err)
	}

	store2, err := persistence.NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to restart store for wal replay: %v", err)
	}

	if !store2.Exists("e2e-wal-user") {
		t.Fatal("expected user to be recovered from WAL replay")
	}

	loaded, err := store2.Load("e2e-wal-user")
	if err != nil {
		t.Fatalf("expected recovered matrix to load: %v", err)
	}
	if len(loaded.Neurons) != 1 {
		t.Fatalf("expected one recovered neuron, got %d", len(loaded.Neurons))
	}
}

func TestE2EDurability_CheckpointManifestRestart(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qubicdb-e2e-durability-checkpoint-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	durability := persistence.DurabilityConfig{
		WALEnabled:    true,
		FsyncPolicy:   persistence.FsyncPolicyInterval,
		FsyncInterval: 10 * time.Millisecond,
	}

	store1, err := persistence.NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to create first store: %v", err)
	}

	pool1 := concurrency.NewWorkerPool(store1, core.DefaultBounds())
	worker1, err := pool1.GetOrCreate("e2e-checkpoint-user")
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	if _, err := worker1.Submit(&concurrency.Operation{
		Type:    concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{Content: "checkpoint survives restart"},
	}); err != nil {
		t.Fatalf("write operation failed: %v", err)
	}

	if err := pool1.PersistAll(); err != nil {
		t.Fatalf("persist all failed: %v", err)
	}
	if err := pool1.Shutdown(); err != nil {
		t.Fatalf("first pool shutdown failed: %v", err)
	}

	store2, err := persistence.NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to restart store for checkpoint replay: %v", err)
	}

	pool2 := concurrency.NewWorkerPool(store2, core.DefaultBounds())
	defer pool2.Shutdown()

	worker2, err := pool2.GetOrCreate("e2e-checkpoint-user")
	if err != nil {
		t.Fatalf("failed to load worker after restart: %v", err)
	}

	result, err := worker2.Submit(&concurrency.Operation{
		Type: concurrency.OpSearch,
		Payload: concurrency.SearchRequest{
			Query: "checkpoint",
			Depth: 1,
			Limit: 10,
		},
	})
	if err != nil {
		t.Fatalf("search after restart failed: %v", err)
	}

	neurons := result.([]*core.Neuron)
	if len(neurons) == 0 {
		t.Fatal("expected persisted content after restart")
	}
}

func TestE2EDurability_StartupRepairRemovesCorruptFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qubicdb-e2e-durability-repair-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	durability := persistence.DurabilityConfig{
		WALEnabled:    false,
		FsyncPolicy:   persistence.FsyncPolicyOff,
		FsyncInterval: time.Second,
		StartupRepair: true,
	}

	store1, err := persistence.NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	m := core.NewMatrix("repair-e2e-user", core.DefaultBounds())
	if err := store1.Save(m); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	userPath := filepath.Join(tmpDir, "data", "repair-e2e-user.nrdb")
	if err := os.WriteFile(userPath, []byte("broken-data"), 0644); err != nil {
		t.Fatalf("failed to corrupt matrix file: %v", err)
	}

	store2, err := persistence.NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to restart store with startup repair: %v", err)
	}

	if store2.Exists("repair-e2e-user") {
		t.Fatal("expected corrupt index to be repaired away on startup")
	}
	if _, err := os.Stat(userPath); !os.IsNotExist(err) {
		t.Fatalf("expected corrupt file to be removed by startup repair, stat err=%v", err)
	}
}
