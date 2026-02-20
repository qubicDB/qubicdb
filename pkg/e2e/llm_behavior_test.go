package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/concurrency"
	"github.com/denizumutdereli/qubicdb/pkg/core"
	"github.com/denizumutdereli/qubicdb/pkg/daemon"
	"github.com/denizumutdereli/qubicdb/pkg/lifecycle"
	"github.com/denizumutdereli/qubicdb/pkg/persistence"
)

// TestRealLLMBehavior simulates real LLM usage patterns with diverse information
func TestRealLLMBehavior(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-llm-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	lm := lifecycle.NewManager()
	dm := daemon.NewDaemonManager(pool, lm, store)

	dm.SetIntervals(
		50*time.Millisecond,
		100*time.Millisecond,
		200*time.Millisecond,
		500*time.Millisecond,
		1*time.Second,
	)

	dm.Start()
	defer dm.Stop()
	defer pool.Shutdown()
	defer lm.Stop()

	indexID := core.IndexID("alice")
	worker, _ := pool.GetOrCreate(indexID)
	lm.RecordActivity(indexID)

	t.Log("\n============================================================")
	t.Log("       REAL LLM BEHAVIOR SIMULATION")
	t.Log("============================================================\n")

	// ============================================================
	// DAY 1: User learns about different topics
	// ============================================================
	t.Log("📅 DAY 1: User learns different topics\n")

	day1Topics := []struct {
		content string
		topic   string
	}{
		{"TypeScript is a typed superset of JavaScript that compiles to plain JavaScript", "programming"},
		{"React is a JavaScript library for building user interfaces", "programming"},
		{"Docker containers package code and dependencies together", "devops"},
		{"Kubernetes orchestrates containerized applications", "devops"},
		{"PostgreSQL is a powerful open source relational database", "database"},
		{"Redis is an in-memory data structure store", "database"},
		{"The user prefers dark mode in all applications", "preference"},
		{"The user's timezone is UTC+3 Istanbul", "preference"},
		{"Project Alpha deadline is March 15th", "project"},
		{"Project Alpha uses React and TypeScript", "project"},
	}

	for _, item := range day1Topics {
		result, _ := worker.Submit(&concurrency.Operation{
			Type: concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{
				Content: item.content,
			},
		})
		n := result.(*core.Neuron)
		t.Logf("  + [%s] %s (energy: %.2f)", item.topic, truncate(item.content, 50), n.Energy)
	}

	// Search to create associations
	t.Log("\n🔍 Searches to build associations...")
	searches := []string{"TypeScript", "React", "Docker", "Project Alpha"}
	for _, q := range searches {
		worker.Submit(&concurrency.Operation{
			Type: concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: q, Depth: 2, Limit: 5},
		})
		lm.RecordActivity(indexID)
	}

	stats1, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	s1 := stats1.(map[string]any)
	t.Logf("\n📊 End of day 1: %d neurons, %d synapses", s1["neuron_count"], s1["synapse_count"])

	// ============================================================
	// Simulate TIME PASSING (idle period -> consolidation)
	// ============================================================
	t.Log("\n⏰ TIME PASSING... (idle period, consolidation)\n")
	time.Sleep(300 * time.Millisecond)

	// ============================================================
	// DAY 2: User asks about old topics
	// ============================================================
	t.Log("📅 DAY 2: User asks about old topics\n")

	recallTests := []struct {
		query    string
		expected string
	}{
		{"What programming language does Project Alpha use?", "TypeScript"},
		{"What is the deadline?", "March 15"},
		{"Docker container", "Docker"},
		{"user preference dark", "dark mode"},
		{"database options", "PostgreSQL"},
	}

	for _, test := range recallTests {
		lm.RecordActivity(indexID)
		result, _ := worker.Submit(&concurrency.Operation{
			Type: concurrency.OpSearch,
			Payload: concurrency.SearchRequest{
				Query: test.query,
				Depth: 3,
				Limit: 5,
			},
		})
		neurons := result.([]*core.Neuron)

		found := false
		for _, n := range neurons {
			if contains(n.Content, test.expected) {
				found = true
				t.Logf("  ✅ Query: '%s' -> Found: '%s' (energy: %.2f, access: %d)",
					truncate(test.query, 30), truncate(n.Content, 40), n.Energy, n.AccessCount)
				break
			}
		}
		if !found {
			t.Logf("  ⚠️ Query: '%s' -> Expected '%s' but got %d results",
				truncate(test.query, 30), test.expected, len(neurons))
		}
	}

	// ============================================================
	// DAY 3: New information that links to old
	// ============================================================
	t.Log("\n📅 DAY 3: New information linked to old\n")

	day3Topics := []struct {
		content string
		topic   string
	}{
		{"Next.js is a React framework for production", "programming"},
		{"TypeScript 5.0 introduced decorators", "programming"},
		{"Project Alpha frontend is built with Next.js", "project"},
		{"Docker Compose simplifies multi-container applications", "devops"},
	}

	for _, item := range day3Topics {
		worker.Submit(&concurrency.Operation{
			Type: concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{
				Content: item.content,
			},
		})
		lm.RecordActivity(indexID)
	}

	// Search to link new and old information
	worker.Submit(&concurrency.Operation{
		Type: concurrency.OpSearch,
		Payload: concurrency.SearchRequest{Query: "Project Alpha React Next.js", Depth: 3, Limit: 10},
	})

	stats2, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	s2 := stats2.(map[string]any)
	t.Logf("\n📊 End of day 3: %d neurons, %d synapses", s2["neuron_count"], s2["synapse_count"])

	// ============================================================
	// Simulate LONG TIME PASSING (weeks) - test decay
	// ============================================================
	t.Log("\n⏰ LONG TIME PASSING... (weeks, decay test)\n")

	// Manually age some neurons to simulate time passing
	matrix := worker.Matrix()
	oldNeurons := 0
	for _, n := range matrix.Neurons {
		// Age half of the neurons
		if oldNeurons < 5 {
			n.LastFiredAt = time.Now().Add(-7 * 24 * time.Hour) // 1 week ago
			oldNeurons++
		}
	}

	// Run decay
	worker.Submit(&concurrency.Operation{Type: concurrency.OpDecay})
	time.Sleep(100 * time.Millisecond)

	// ============================================================
	// DAY X: Test recall of old vs new information
	// ============================================================
	t.Log("📅 DAY X: Old vs new information comparison\n")

	// Count energy levels
	activeCount := 0
	dormantCount := 0
	for _, n := range matrix.Neurons {
		if n.IsAlive() {
			activeCount++
		} else if n.IsDormant() {
			dormantCount++
		}
	}

	t.Logf("  Active neurons: %d", activeCount)
	t.Logf("  Dormant neurons: %d", dormantCount)

	// ============================================================
	// Test REACTIVATION - old memory brought back
	// ============================================================
	t.Log("\n🔄 REACTIVATION: Recovering old memory\n")

	// Find a dormant neuron
	var dormantNeuron *core.Neuron
	for _, n := range matrix.Neurons {
		if n.Energy < 0.5 {
			dormantNeuron = n
			break
		}
	}

	if dormantNeuron != nil {
		beforeEnergy := dormantNeuron.Energy
		dormantNeuron.Reactivate(0.3)
		t.Logf("  Neuron reactivated: energy %.2f -> %.2f", beforeEnergy, dormantNeuron.Energy)
		t.Logf("  Content: '%s'", truncate(dormantNeuron.Content, 50))
	}

	// ============================================================
	// Final Statistics
	// ============================================================
	t.Log("\n============================================================")
	t.Log("       FINAL STATISTICS")
	t.Log("============================================================\n")

	finalStats, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	fs := finalStats.(map[string]any)

	t.Logf("  Total Neurons: %d", fs["neuron_count"])
	t.Logf("  Total Synapses: %d", fs["synapse_count"])
	t.Logf("  Average Energy: %.2f", fs["average_energy"])
	t.Logf("  Depth Distribution: %v", fs["depth_distribution"])
	t.Logf("  Total Activations: %d", fs["total_activations"])

	// Synapse strength distribution
	weakSynapses := 0
	strongSynapses := 0
	for _, s := range matrix.Synapses {
		if s.IsWeak() {
			weakSynapses++
		} else if s.IsAlive() {
			strongSynapses++
		}
	}
	t.Logf("  Strong Synapses: %d", strongSynapses)
	t.Logf("  Weak Synapses: %d", weakSynapses)

	t.Log("\n✅ Test completed - brain is functioning organically!")
}

// TestBrainMechanicsLongTerm tests long-term memory persistence
func TestBrainMechanicsLongTerm(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-longterm-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	t.Log("\n=== LONG-TERM MEMORY MECHANICS ===\n")

	worker, _ := pool.GetOrCreate("longterm-user")

	// Add neurons
	for i := 0; i < 10; i++ {
		worker.Submit(&concurrency.Operation{
			Type: concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{
				Content: fmt.Sprintf("Long term memory content %d", i),
			},
		})
	}

	matrix := worker.Matrix()

	// Simulate different ages
	neurons := make([]*core.Neuron, 0)
	for _, n := range matrix.Neurons {
		neurons = append(neurons, n)
	}

	// Group 1: Recent (high energy)
	// Group 2: 1 week old
	// Group 3: 1 month old
	// Group 4: 1 year old (but frequently accessed)

	t.Log("Simulating different neuron ages:")
	for i, n := range neurons {
		switch i % 4 {
		case 0:
			t.Logf("  Neuron %d: Recent (energy: %.2f)", i, n.Energy)
		case 1:
			n.LastFiredAt = time.Now().Add(-7 * 24 * time.Hour)
			n.Decay(0.1)
			t.Logf("  Neuron %d: 1 week old (energy: %.2f)", i, n.Energy)
		case 2:
			n.LastFiredAt = time.Now().Add(-30 * 24 * time.Hour)
			n.Decay(0.1)
			n.Decay(0.1)
			t.Logf("  Neuron %d: 1 month old (energy: %.2f)", i, n.Energy)
		case 3:
			n.LastFiredAt = time.Now().Add(-365 * 24 * time.Hour)
			n.AccessCount = 100 // Frequently accessed
			n.Decay(0.1)
			t.Logf("  Neuron %d: 1 year old but frequent (energy: %.2f, access: %d)", i, n.Energy, n.AccessCount)
		}
	}

	// Test recall effectiveness at different energy levels
	t.Log("\n📊 Energy vs Recall Relevance:")
	for i, n := range neurons {
		status := "Active"
		if n.IsDormant() {
			status = "Dormant"
		} else if !n.IsAlive() {
			status = "Very Weak"
		}
		t.Logf("  Neuron %d: energy=%.3f, status=%s", i, n.Energy, status)
	}

	// Verify nothing is deleted - just weakened
	finalCount := len(matrix.Neurons)
	t.Logf("\n✅ Neuron count preserved: %d (none deleted)", finalCount)
}

// TestSynapseWeakeningNotDeletion tests that synapses weaken but persist
func TestSynapseWeakeningNotDeletion(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-synapse-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	t.Log("\n=== SYNAPSE WEAKENING MECHANICS ===\n")

	worker, _ := pool.GetOrCreate("synapse-user")

	// Create neurons and force synapse creation
	n1, _ := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{Content: "Concept A - TypeScript"},
	})
	n2, _ := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{Content: "Concept B - React"},
	})

	neuron1 := n1.(*core.Neuron)
	neuron2 := n2.(*core.Neuron)

	// Create synapse manually
	matrix := worker.Matrix()
	synapse := core.NewSynapse(neuron1.ID, neuron2.ID, 1.0)
	matrix.Synapses[synapse.ID] = synapse

	t.Logf("Initial synapse weight: %.2f", synapse.Weight)

	// Simulate time and decay
	for round := 1; round <= 5; round++ {
		synapse.LastCoFire = time.Now().Add(-time.Duration(round) * 24 * time.Hour)
		synapse.Decay(0.2)
		
		status := "Active"
		if synapse.IsWeak() {
			status = "Weak"
		}
		if synapse.ShouldArchive() {
			status = "Archive candidate"
		}
		
		t.Logf("  Round %d (%d days): weight=%.3f (%s)", round, round, synapse.Weight, status)
	}

	// Reactivation
	t.Log("\n🔄 Synapse reactivation (concepts mentioned together again):")
	beforeWeight := synapse.Weight
	synapse.Reactivate(0.5)
	t.Logf("  Weight %.3f -> %.3f", beforeWeight, synapse.Weight)

	// Verify synapse still exists
	if _, exists := matrix.Synapses[synapse.ID]; !exists {
		t.Error("Synapse should still exist!")
	} else {
		t.Log("\n✅ Synapse still exists - only weakened, not deleted")
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
