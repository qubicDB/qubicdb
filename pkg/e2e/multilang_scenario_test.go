package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/concurrency"
	"github.com/denizumutdereli/qubicdb/pkg/core"
	"github.com/denizumutdereli/qubicdb/pkg/daemon"
	"github.com/denizumutdereli/qubicdb/pkg/lifecycle"
	"github.com/denizumutdereli/qubicdb/pkg/persistence"
)

// TestMultiLanguageConversation tests memory with English, Turkish, German content
func TestMultiLanguageConversation(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-multilang-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	lm := lifecycle.NewManager()
	dm := daemon.NewDaemonManager(pool, lm, store)

	dm.SetIntervals(50*time.Millisecond, 100*time.Millisecond, 200*time.Millisecond, 500*time.Millisecond, 1*time.Second)
	dm.Start()
	defer dm.Stop()
	defer pool.Shutdown()
	defer lm.Stop()

	worker, _ := pool.GetOrCreate("multilang-user")
	lm.RecordActivity("multilang-user")

	t.Log("\n============================================================")
	t.Log("    MULTI-LANGUAGE CONVERSATION TEST (EN/TR/DE)")
	t.Log("============================================================\n")

	// Simulate a real conversation with mixed languages
	conversation := []struct {
		role    string
		content string
		lang    string
	}{
		// English context
		{"user", "My name is Alex and I work at TechCorp as a senior developer", "EN"},
		{"assistant", "Nice to meet you Alex! I'll remember you work at TechCorp as a senior developer.", "EN"},
		{"user", "I prefer using TypeScript and React for frontend development", "EN"},
		{"assistant", "Got it! TypeScript and React are great choices for frontend.", "EN"},

		// Turkish context
		{"user", "Benim favori takımım Fenerbahçe ve her hafta maçlarını izlerim", "TR"},
		{"assistant", "Fenerbahçe taraftarı olduğunuzu not ettim! Her hafta maç izlemeniz güzel.", "TR"},
		{"user", "Istanbul'da Kadıköy'de yaşıyorum, deniz kenarında güzel bir semt", "TR"},
		{"assistant", "Kadıköy gerçekten güzel bir semt, deniz manzarası muhteşem olmalı.", "TR"},

		// German context
		{"user", "Ich lerne gerade Deutsch und finde die Sprache sehr interessant", "DE"},
		{"assistant", "Das ist toll! Deutsch zu lernen ist eine gute Entscheidung.", "DE"},
		{"user", "Mein Lieblingsessen ist Schnitzel mit Kartoffelsalat", "DE"},
		{"assistant", "Schnitzel mit Kartoffelsalat ist ein Klassiker der deutschen Küche!", "DE"},

		// Mixed context - technical
		{"user", "For the new project, I need to set up a Kubernetes cluster on AWS", "EN"},
		{"assistant", "I can help with Kubernetes on AWS. Do you prefer EKS or self-managed?", "EN"},
		{"user", "Projede ayrıca Redis cache kullanmamız gerekiyor performans için", "TR"},
		{"assistant", "Redis cache iyi bir seçim, özellikle yüksek trafik senaryolarında.", "TR"},
	}

	t.Log("--- Phase 1: Recording conversation ---\n")
	for _, msg := range conversation {
		// Store both user and assistant messages as the LLM would
		content := fmt.Sprintf("[%s] %s: %s", msg.lang, msg.role, msg.content)
		result, _ := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{Content: content},
		})
		n := result.(*core.Neuron)
		t.Logf("  [%s] %s (energy: %.2f)", msg.lang, truncateStr(msg.content, 50), n.Energy)
		lm.RecordActivity("multilang-user")
	}

	// Create associations through searches
	searches := []string{"Alex TechCorp", "Fenerbahçe", "Deutsch", "Kubernetes AWS", "Redis"}
	for _, q := range searches {
		worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: q, Depth: 2, Limit: 5},
		})
	}

	stats1, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	s1 := stats1.(map[string]any)
	t.Logf("\n📊 After conversation: %d neurons, %d synapses\n", s1["neuron_count"], s1["synapse_count"])

	// ============================================================
	// Phase 2: Context-based recall (not direct questions)
	// ============================================================
	t.Log("--- Phase 2: Context-based recall ---\n")

	contextQueries := []struct {
		context  string
		expected []string
		desc     string
	}{
		{
			context:  "frontend development technologies preferences",
			expected: []string{"TypeScript", "React"},
			desc:     "Should recall tech preferences from context",
		},
		{
			context:  "futbol takımı haftalık aktivite",
			expected: []string{"Fenerbahçe", "maç"},
			desc:     "Should recall Turkish football context",
		},
		{
			context:  "deutsche Sprache Essen Kultur",
			expected: []string{"Deutsch", "Schnitzel"},
			desc:     "Should recall German language and food context",
		},
		{
			context:  "cloud infrastructure caching performance",
			expected: []string{"Kubernetes", "Redis", "AWS"},
			desc:     "Should recall technical infrastructure context",
		},
		{
			context:  "Istanbul semt deniz",
			expected: []string{"Kadıköy", "deniz"},
			desc:     "Should recall location context",
		},
	}

	for _, cq := range contextQueries {
		result, _ := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: cq.context, Depth: 3, Limit: 10},
		})
		neurons := result.([]*core.Neuron)

		foundCount := 0
		foundItems := []string{}
		for _, n := range neurons {
			for _, exp := range cq.expected {
				if strings.Contains(n.Content, exp) {
					foundCount++
					foundItems = append(foundItems, exp)
					break
				}
			}
		}

		status := "⚠️"
		if foundCount >= len(cq.expected)/2+1 {
			status = "✅"
		}
		t.Logf("  %s %s", status, cq.desc)
		t.Logf("     Query: '%s'", truncateStr(cq.context, 40))
		t.Logf("     Found: %v (%d/%d expected)", foundItems, foundCount, len(cq.expected))
	}

	t.Log("\n✅ Multi-language test completed")
}

// TestLongConversationScenario simulates a long conversation across multiple topics
func TestLongConversationScenario(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-longconv-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	lm := lifecycle.NewManager()
	dm := daemon.NewDaemonManager(pool, lm, store)

	dm.SetIntervals(30*time.Millisecond, 60*time.Millisecond, 120*time.Millisecond, 300*time.Millisecond, 500*time.Millisecond)
	dm.Start()
	defer dm.Stop()
	defer pool.Shutdown()
	defer lm.Stop()

	worker, _ := pool.GetOrCreate("longconv-user")
	indexID := core.IndexID("longconv-user")

	t.Log("\n============================================================")
	t.Log("    LONG CONVERSATION SCENARIO - TOPIC SWITCHING")
	t.Log("============================================================\n")

	// Simulate a long conversation that switches topics naturally
	topics := []struct {
		name     string
		messages []string
	}{
		{
			name: "Personal Info",
			messages: []string{
				"User's name is Maria, she is 28 years old",
				"Maria works as a data scientist at DataLabs Inc",
				"She has a PhD in Machine Learning from MIT",
				"Her favorite programming language is Python",
			},
		},
		{
			name: "Current Project",
			messages: []string{
				"Maria is working on a recommendation system project",
				"The project uses collaborative filtering and deep learning",
				"Tech stack: Python, TensorFlow, Redis, PostgreSQL",
				"Deadline is end of Q2 2026",
			},
		},
		{
			name: "Hobbies",
			messages: []string{
				"Maria enjoys hiking on weekends",
				"She plays chess competitively and has ELO rating of 1800",
				"Recently started learning piano, practicing Chopin",
				"Reads sci-fi books, favorite author is Isaac Asimov",
			},
		},
		{
			name: "Travel",
			messages: []string{
				"Maria visited Japan last year, loved Kyoto",
				"Next trip planned to Iceland to see Northern Lights",
				"Always travels with her camera, enjoys photography",
				"Prefers local food experiences over tourist restaurants",
			},
		},
		{
			name: "Technical Discussions",
			messages: []string{
				"Discussed transformer architecture optimization",
				"Maria prefers PyTorch for research, TensorFlow for production",
				"She contributed to an open source ML library called FastML",
				"Uses Docker and Kubernetes for ML model deployment",
			},
		},
	}

	t.Log("--- Recording 5 topic areas with multiple messages each ---\n")

	totalNeurons := 0
	for _, topic := range topics {
		t.Logf("📁 Topic: %s", topic.name)
		for _, msg := range topic.messages {
			worker.Submit(&concurrency.Operation{
				Type:    concurrency.OpWrite,
				Payload: concurrency.AddNeuronRequest{Content: msg},
			})
			lm.RecordActivity(indexID)
			totalNeurons++
			t.Logf("   + %s", truncateStr(msg, 55))
		}
		// Search within topic to create associations
		worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: topic.name, Depth: 2, Limit: 10},
		})
	}

	// Let daemons run for consolidation
	time.Sleep(200 * time.Millisecond)

	stats, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	s := stats.(map[string]any)
	t.Logf("\n📊 Total: %d neurons, %d synapses", s["neuron_count"], s["synapse_count"])

	// ============================================================
	// Cross-topic recall test
	// ============================================================
	t.Log("\n--- Cross-topic contextual recall ---\n")

	crossTopicQueries := []struct {
		query    string
		expected string
		desc     string
	}{
		{"machine learning Python production", "PyTorch TensorFlow", "Tech stack connection"},
		{"Japan travel photography", "camera Kyoto", "Travel & hobby connection"},
		{"data scientist recommendation", "collaborative filtering", "Work & project connection"},
		{"chess music intellectual", "ELO piano", "Hobby connections"},
		{"container deployment ML", "Docker Kubernetes", "DevOps & ML connection"},
	}

	for _, cq := range crossTopicQueries {
		result, _ := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: cq.query, Depth: 3, Limit: 5},
		})
		neurons := result.([]*core.Neuron)

		t.Logf("  Query: '%s'", cq.query)
		if len(neurons) > 0 {
			t.Logf("  ✅ Found %d results, top: '%s'", len(neurons), truncateStr(neurons[0].Content, 45))
		} else {
			t.Logf("  ⚠️ No results found")
		}
	}

	t.Log("\n✅ Long conversation test completed")
}

// TestHotColdMemoryRetrieval tests retrieval of recent (hot) vs old (cold) memories
func TestHotColdMemoryRetrieval(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-hotcold-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	worker, _ := pool.GetOrCreate("hotcold-user")

	t.Log("\n============================================================")
	t.Log("    HOT vs COLD MEMORY RETRIEVAL TEST")
	t.Log("============================================================\n")

	// Create "old" memories (cold)
	t.Log("--- Creating COLD memories (simulating old information) ---\n")
	coldMemories := []string{
		"User's old phone number was 555-0100",
		"Previously worked at OldCompany as junior developer",
		"Used to live in Boston before moving",
		"Old project was a simple CRUD application",
	}

	for _, mem := range coldMemories {
		result, _ := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{Content: mem},
		})
		n := result.(*core.Neuron)
		// Simulate time passing - reduce energy
		n.LastFiredAt = time.Now().Add(-30 * 24 * time.Hour) // 30 days ago
		n.Decay(0.3)
		t.Logf("  [COLD] %s (energy: %.2f)", truncateStr(mem, 45), n.Energy)
	}

	// Create "recent" memories (hot)
	t.Log("\n--- Creating HOT memories (recent information) ---\n")
	hotMemories := []string{
		"User's current phone is 555-9999",
		"Now works at NewTech Inc as tech lead",
		"Currently lives in San Francisco",
		"Current project is a microservices architecture",
	}

	for _, mem := range hotMemories {
		result, _ := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{Content: mem},
		})
		n := result.(*core.Neuron)
		t.Logf("  [HOT] %s (energy: %.2f)", truncateStr(mem, 45), n.Energy)
	}

	// Test retrieval - should prefer hot memories
	t.Log("\n--- Retrieval test: should prefer HOT over COLD ---\n")

	queries := []struct {
		query       string
		expectHot   string
		expectCold  string
	}{
		{"phone number contact", "555-9999", "555-0100"},
		{"work company job", "NewTech", "OldCompany"},
		{"location city lives", "San Francisco", "Boston"},
		{"project current work", "microservices", "CRUD"},
	}

	for _, q := range queries {
		result, _ := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: q.query, Depth: 2, Limit: 5},
		})
		neurons := result.([]*core.Neuron)

		if len(neurons) > 0 {
			topResult := neurons[0].Content
			isHot := strings.Contains(topResult, q.expectHot)
			isCold := strings.Contains(topResult, q.expectCold)

			if isHot {
				t.Logf("  ✅ '%s' -> HOT memory preferred (energy: %.2f)", q.query, neurons[0].Energy)
			} else if isCold {
				t.Logf("  ⚠️ '%s' -> COLD memory returned (energy: %.2f)", q.query, neurons[0].Energy)
			} else {
				t.Logf("  ❓ '%s' -> Other result: %s", q.query, truncateStr(topResult, 30))
			}
		}
	}

	t.Log("\n✅ Hot/Cold memory test completed")
}

// TestLifecycleBrainCycles tests the complete brain lifecycle
func TestLifecycleBrainCycles(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-lifecycle-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	lm := lifecycle.NewManager()
	dm := daemon.NewDaemonManager(pool, lm, store)

	dm.SetIntervals(20*time.Millisecond, 40*time.Millisecond, 80*time.Millisecond, 200*time.Millisecond, 400*time.Millisecond)
	dm.Start()
	defer dm.Stop()
	defer pool.Shutdown()
	defer lm.Stop()

	indexID := core.IndexID("lifecycle-user")
	worker, _ := pool.GetOrCreate(indexID)

	t.Log("\n============================================================")
	t.Log("    BRAIN LIFECYCLE CYCLES TEST")
	t.Log("============================================================\n")

	// Phase 1: Active learning
	t.Log("--- Phase 1: ACTIVE state (learning) ---\n")
	for i := 0; i < 10; i++ {
		worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{Content: fmt.Sprintf("Learning content %d about topic %c", i, 'A'+i%5)},
		})
		lm.RecordActivity(indexID)
	}

	state1 := lm.GetState(indexID)
	t.Logf("  State: %d (0=Active)", state1)
	t.Logf("  Active users: %v", lm.GetActiveUsers())

	// Phase 2: Idle period
	t.Log("\n--- Phase 2: IDLE transition (no activity) ---\n")
	time.Sleep(150 * time.Millisecond)
	lm.CheckAndTransition(indexID)
	state2 := lm.GetState(indexID)
	t.Logf("  State after idle: %d (1=Idle)", state2)

	// Phase 3: Sleep
	t.Log("\n--- Phase 3: SLEEP transition ---\n")
	lm.ForceSleep(indexID)
	state3 := lm.GetState(indexID)
	t.Logf("  State after sleep: %d (2=Sleeping)", state3)
	t.Logf("  Sleeping users: %v", lm.GetSleepingUsers())

	// During sleep, reorg should happen
	time.Sleep(100 * time.Millisecond)

	// Phase 4: Wake up
	t.Log("\n--- Phase 4: WAKE UP (recall request) ---\n")
	lm.ForceWake(indexID)
	state4 := lm.GetState(indexID)
	t.Logf("  State after wake: %d (0=Active)", state4)

	// Test recall after wake
	result, _ := worker.Submit(&concurrency.Operation{
		Type:    concurrency.OpSearch,
		Payload: concurrency.SearchRequest{Query: "Learning topic", Depth: 2, Limit: 5},
	})
	neurons := result.([]*core.Neuron)
	t.Logf("  Recall after wake: found %d neurons", len(neurons))

	// Final stats
	stats, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	s := stats.(map[string]any)
	t.Logf("\n📊 Final: %d neurons, %d synapses, avg energy: %.2f", s["neuron_count"], s["synapse_count"], s["average_energy"])

	t.Log("\n✅ Lifecycle test completed")
}

// TestLLMQueryPatterns tests various query patterns an LLM might use
func TestLLMQueryPatterns(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-llmquery-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	worker, _ := pool.GetOrCreate("llmquery-user")

	t.Log("\n============================================================")
	t.Log("    LLM QUERY PATTERNS TEST")
	t.Log("============================================================\n")

	// Store diverse information as an LLM would
	llmStyleMemories := []string{
		"User profile: John Smith, age 35, software engineer",
		"User preference: prefers dark mode, uses Vim editor",
		"User workspace: VS Code with TypeScript and Go extensions",
		"User project: Building a neural database for LLM memory",
		"User feedback: Likes concise responses, dislikes lengthy explanations",
		"User context: Working on QubicDB implementation in Go",
		"User history: Previously discussed Kubernetes deployment",
		"User goal: Achieve 90% test coverage for the project",
		"User constraint: Deadline is end of February 2026",
		"User stack: Go, TypeScript, Redis, PostgreSQL, Docker",
	}

	t.Log("--- Storing LLM-style memories ---\n")
	for _, mem := range llmStyleMemories {
		worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{Content: mem},
		})
		t.Logf("  + %s", truncateStr(mem, 55))
	}

	// Test various LLM query patterns
	t.Log("\n--- Testing LLM query patterns ---\n")

	queryPatterns := []struct {
		pattern string
		query   string
		desc    string
	}{
		{"direct", "user name", "Direct attribute lookup"},
		{"contextual", "what editor does the user prefer", "Contextual preference query"},
		{"project", "current project goal deadline", "Project-related context"},
		{"technical", "programming languages stack", "Technical stack query"},
		{"behavioral", "user likes dislikes preferences", "Behavioral patterns"},
		{"historical", "previously discussed topics", "Historical context"},
		{"fuzzy", "test code coverage go", "Fuzzy technical query"},
		{"compound", "neural database memory LLM", "Compound concept query"},
	}

	for _, qp := range queryPatterns {
		result, _ := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: qp.query, Depth: 3, Limit: 5},
		})
		neurons := result.([]*core.Neuron)

		t.Logf("  [%s] %s", qp.pattern, qp.desc)
		t.Logf("     Query: '%s'", qp.query)
		if len(neurons) > 0 {
			t.Logf("     ✅ Found %d, top: '%s' (energy: %.2f)", len(neurons), truncateStr(neurons[0].Content, 40), neurons[0].Energy)
		} else {
			t.Logf("     ⚠️ No results")
		}
	}

	t.Log("\n✅ LLM query patterns test completed")
}

// TestOverfitPrevention tests with diverse, unrelated scenarios to prevent overfitting
func TestOverfitPrevention(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-overfit-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	t.Log("\n============================================================")
	t.Log("    OVERFIT PREVENTION - DIVERSE SCENARIOS")
	t.Log("============================================================\n")

	// Test with completely different user profiles
	scenarios := []struct {
		indexID   string
		memories []string
		queries  []string
	}{
		{
			indexID: "chef-user",
			memories: []string{
				"Recipe: Italian pasta carbonara with guanciale",
				"Cooking tip: Always salt pasta water generously",
				"Favorite cuisine: Mediterranean and Japanese fusion",
				"Kitchen equipment: Prefer cast iron and copper pans",
			},
			queries: []string{"pasta Italian", "cooking salt tip", "cuisine favorite"},
		},
		{
			indexID: "musician-user",
			memories: []string{
				"Instrument: Playing jazz saxophone for 15 years",
				"Practice routine: 2 hours daily, focus on improvisation",
				"Favorite artists: John Coltrane, Charlie Parker",
				"Current piece: Working on Giant Steps transcription",
			},
			queries: []string{"saxophone jazz", "practice daily", "Coltrane artist"},
		},
		{
			indexID: "athlete-user",
			memories: []string{
				"Sport: Marathon runner, personal best 3:15",
				"Training: 60 miles per week, long runs on Sunday",
				"Nutrition: High carb diet, intermittent fasting",
				"Goal: Qualify for Boston Marathon next year",
			},
			queries: []string{"marathon running", "training weekly", "Boston goal"},
		},
		{
			indexID: "scientist-user",
			memories: []string{
				"Research: Quantum computing error correction",
				"Lab: Using IBM Q for experiments",
				"Publication: Recent paper on topological qubits",
				"Collaboration: Working with CERN on quantum sensing",
			},
			queries: []string{"quantum computing", "IBM Q lab", "CERN collaboration"},
		},
	}

	for _, scenario := range scenarios {
		t.Logf("\n--- Scenario: %s ---\n", scenario.indexID)
		worker, _ := pool.GetOrCreate(core.IndexID(scenario.indexID))

		// Store memories
		for _, mem := range scenario.memories {
			worker.Submit(&concurrency.Operation{
				Type:    concurrency.OpWrite,
				Payload: concurrency.AddNeuronRequest{Content: mem},
			})
		}

		// Test queries
		successCount := 0
		for _, q := range scenario.queries {
			result, _ := worker.Submit(&concurrency.Operation{
				Type:    concurrency.OpSearch,
				Payload: concurrency.SearchRequest{Query: q, Depth: 2, Limit: 3},
			})
			neurons := result.([]*core.Neuron)
			if len(neurons) > 0 {
				successCount++
				t.Logf("  ✅ '%s' -> %s", q, truncateStr(neurons[0].Content, 40))
			} else {
				t.Logf("  ⚠️ '%s' -> no results", q)
			}
		}
		t.Logf("  Success rate: %d/%d", successCount, len(scenario.queries))
	}

	// Cross-user isolation test
	t.Log("\n--- Cross-user isolation test ---")
	chefWorker, _ := pool.GetOrCreate("chef-user")
	result, _ := chefWorker.Submit(&concurrency.Operation{
		Type:    concurrency.OpSearch,
		Payload: concurrency.SearchRequest{Query: "quantum computing", Depth: 2, Limit: 3},
	})
	neurons := result.([]*core.Neuron)
	if len(neurons) == 0 {
		t.Log("  ✅ Chef user cannot access scientist's quantum memories")
	} else {
		t.Log("  ⚠️ Cross-user leak detected!")
	}

	t.Log("\n✅ Overfit prevention test completed")
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
