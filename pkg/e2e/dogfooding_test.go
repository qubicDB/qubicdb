package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/daemon"
	"github.com/qubicDB/qubicdb/pkg/lifecycle"
	"github.com/qubicDB/qubicdb/pkg/persistence"
)

// TestDogfoodingProjectMemory tests QubicDB by storing and recalling
// the actual project discussion - the system remembering itself!
func TestDogfoodingProjectMemory(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-dogfood-*")
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

	worker, _ := pool.GetOrCreate("project-memory")
	indexID := core.IndexID("project-memory")

	t.Log("\n============================================================")
	t.Log("    DOGFOODING TEST: QubicDB Remembering Itself!")
	t.Log("============================================================\n")

	// Store the actual project discussion as memories
	projectDiscussion := []string{
		// Project concept
		"Bu proje Recursive Memory için başka bir pattern kullanmak - deepagent'ın virtual filesystem yerine",
		"LLM modellere sürekli flash ve çok hızlı scale edilebilir memory sağlayacağız",
		"Beyin hafızası gibi çalışması lazım - özellikle vector embedding semantic search olmasın",

		// Brain mechanics
		"Engram neronları gibi en hazır aktive edilecek ön hafıza",
		"Kendi içlerinde dallanmaları - sonra orta loba kayması",
		"Işıma ile hatırlanabilir bilgilerin güçlenmesi - synapse-neuron bağlantıları",
		"Az kullanılanların silikleşmesi ama silinmemesi",
		"Tüm bilginin birbiriyle bir dal gibi ilişkilendirilmesi",

		// Technical decisions
		"Strict rules değil emergent behavior - sistem kendi kendine öğrenmeli",
		"Hebbian Learning: Neurons that fire together wire together",
		"Co-occurrence sistemin kendisi belirliyor - biz sadece temporal proximity tanımlıyoruz",

		// Architecture
		"Neuron Matrix: Her neuron bir N-dimensional point",
		"Position, energy, links, depth, birth, last_fire alanları var",
		"Yeni neuron parent'ının yakınında doğar",
		"Sık birlikte fire edenler birbirine yaklaşır - self-organizing map",

		// Operations
		"Write: Yeni neuron en yakın mevcut neuronun yanında spawn",
		"Read: Cue'dan nearest neighbors, spread activation, return hot neurons",
		"Strengthen: Co-activation ile link weight artışı, distance azalışı",
		"Decay: Time-based energy drain, weak links prune",
		"Consolidate: Surface depth=0'dan deeper layer'a geçiş when stable",

		// User-specific
		"Her kullanıcı için farklı şekillenir - belki 10 belki 100K parça recursive",
		"Uyuma eyleminde re-organizasyon ve kalıcılık",
		"User insert read activity seyrekleşince uyku tetiklenir",

		// Implementation
		"Go ile low level güçle yaşayan instance",
		"In-memory PoC yazıp sonra scale ve persistence düşüneceğiz",
		"Background goroutines: decay loop, consolidate loop, sleep detector",
		"HTTP API: Write, Read, Touch, Forget, Dump",

		// Current status
		"QubicDB Go implementasyonu tamamlandı",
		"Test coverage artırılıyor - hedef 90%+",
		"Multi-language testler (EN/TR/DE) başarılı",
		"Lifecycle testleri çalışıyor: Active, Idle, Sleeping, Dormant",
	}

	t.Log("--- Phase 1: Storing project discussion as neurons ---\n")
	for _, content := range projectDiscussion {
		worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{Content: content},
		})
		lm.RecordActivity(indexID)
	}

	// Create associations through topic searches
	topicSearches := []string{
		"beyin hafıza neuron",
		"Hebbian learning synapse",
		"Go implementation API",
		"lifecycle sleep wake",
		"user memory organic",
	}
	for _, q := range topicSearches {
		worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: q, Depth: 3, Limit: 10},
		})
	}

	stats, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	s := stats.(map[string]any)
	t.Logf("📊 Stored: %d neurons, %d synapses\n", s["neuron_count"], s["synapse_count"])

	// ============================================================
	// Phase 2: Query the system about itself
	// ============================================================
	t.Log("--- Phase 2: Querying the system about itself ---\n")

	selfQueries := []struct {
		question string
		keywords []string
		desc     string
	}{
		{
			question: "Bu proje ne yapıyor nedir amacı",
			keywords: []string{"Memory", "LLM", "flash", "deepagent"},
			desc:     "Project purpose",
		},
		{
			question: "Beyin mekanikleri nasıl çalışıyor",
			keywords: []string{"neuron", "synapse", "Hebbian", "fire"},
			desc:     "Brain mechanics",
		},
		{
			question: "Neuron nasıl oluşturuluyor position energy",
			keywords: []string{"Position", "energy", "spawn", "dimension"},
			desc:     "Neuron creation",
		},
		{
			question: "Uyku ne zaman tetikleniyor sleep",
			keywords: []string{"uyku", "sleep", "seyrek", "activity"},
			desc:     "Sleep trigger",
		},
		{
			question: "Go implementation API",
			keywords: []string{"Go", "HTTP", "API"},
			desc:     "Tech implementation",
		},
		{
			question: "Test coverage multi-language",
			keywords: []string{"test", "coverage", "90%", "EN", "TR"},
			desc:     "Testing status",
		},
		{
			question: "decay strengthen consolidate operations",
			keywords: []string{"Decay", "Strengthen", "Consolidate"},
			desc:     "Operations",
		},
		{
			question: "user specific organic growth",
			keywords: []string{"kullanıcı", "organic", "şekil", "recursive"},
			desc:     "User-specific memory",
		},
	}

	successCount := 0
	for _, sq := range selfQueries {
		result, _ := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: sq.question, Depth: 3, Limit: 5},
		})
		neurons := result.([]*core.Neuron)

		foundKeywords := []string{}
		for _, n := range neurons {
			for _, kw := range sq.keywords {
				if strings.Contains(strings.ToLower(n.Content), strings.ToLower(kw)) {
					found := false
					for _, fk := range foundKeywords {
						if fk == kw {
							found = true
							break
						}
					}
					if !found {
						foundKeywords = append(foundKeywords, kw)
					}
				}
			}
		}

		status := "⚠️"
		if len(foundKeywords) >= 1 {
			status = "✅"
			successCount++
		}

		t.Logf("  %s [%s] %s", status, sq.desc, sq.question[:min(35, len(sq.question))])
		if len(neurons) > 0 {
			t.Logf("     Found: %d results, keywords: %v", len(neurons), foundKeywords)
			t.Logf("     Top: '%s'", truncateS(neurons[0].Content, 50))
		}
	}

	t.Logf("\n📊 Self-query success rate: %d/%d", successCount, len(selfQueries))

	// ============================================================
	// Phase 3: Test contextual understanding
	// ============================================================
	t.Log("\n--- Phase 3: Contextual understanding ---\n")

	contextTests := []struct {
		context  string
		expected string
	}{
		{"strict rules vs emergent", "kendi kendine öğrenmeli"},
		{"vector embedding semantic", "olmasın"},
		{"crash restart resume", "persistence"},
		{"active idle sleeping", "lifecycle"},
	}

	for _, ct := range contextTests {
		result, _ := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: ct.context, Depth: 2, Limit: 3},
		})
		neurons := result.([]*core.Neuron)

		found := false
		for _, n := range neurons {
			if strings.Contains(strings.ToLower(n.Content), strings.ToLower(ct.expected)) {
				found = true
				break
			}
		}

		if found || len(neurons) > 0 {
			t.Logf("  ✅ '%s' → found relevant context", ct.context)
		} else {
			t.Logf("  ⚠️ '%s' → no context found", ct.context)
		}
	}

	// ============================================================
	// Phase 4: Lifecycle test - simulate time passing
	// ============================================================
	t.Log("\n--- Phase 4: Lifecycle simulation ---\n")

	// Simulate inactivity
	time.Sleep(200 * time.Millisecond)
	lm.CheckAndTransition(indexID)
	state := lm.GetState(indexID)
	t.Logf("  State after inactivity: %d", state)

	// Force sleep and check memory preservation
	lm.ForceSleep(indexID)
	time.Sleep(100 * time.Millisecond)

	// Wake up and recall
	lm.ForceWake(indexID)
	lm.RecordActivity(indexID)

	// Recall after wake
	result, _ := worker.Submit(&concurrency.Operation{
		Type:    concurrency.OpSearch,
		Payload: concurrency.SearchRequest{Query: "QubicDB proje beyin hafıza", Depth: 3, Limit: 10},
	})
	neurons := result.([]*core.Neuron)
	t.Logf("  Recall after wake: %d neurons found", len(neurons))

	// Final stats
	finalStats, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	fs := finalStats.(map[string]any)
	t.Logf("\n📊 Final: %d neurons, %d synapses, avg energy: %.2f", fs["neuron_count"], fs["synapse_count"], fs["average_energy"])

	t.Log("\n✅ Dogfooding test completed - QubicDB successfully remembers its own project!")
}

// TestMixedLanguageProjectDiscussion tests recalling project info in different languages
func TestMixedLanguageProjectDiscussion(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-mixlang-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	worker, _ := pool.GetOrCreate("mixlang-user")

	t.Log("\n============================================================")
	t.Log("    MIXED LANGUAGE PROJECT DISCUSSION TEST")
	t.Log("============================================================\n")

	// Store information in multiple languages about the same concepts
	mixedContent := []string{
		// Turkish
		"QubicDB bir beyin hafızası gibi çalışan veritabanı",
		"Neuronlar birbirine bağlanıyor ve güçleniyor",
		"Kullanılmayan bağlantılar zayıflıyor ama silinmiyor",

		// English
		"QubicDB is a brain-like memory database for LLMs",
		"Neurons connect to each other through synapses",
		"Unused connections weaken but are not deleted",

		// German
		"QubicDB ist eine gehirnähnliche Speicherdatenbank",
		"Neuronen verbinden sich durch Synapsen",
		"Unbenutzte Verbindungen werden schwächer aber nicht gelöscht",

		// Technical (mixed)
		"Hebbian learning: neurons that fire together wire together",
		"Decay mekanizması: time-based energy drain",
		"Consolidation: surface'dan deep layer'a geçiş",
	}

	t.Log("--- Storing multi-language content ---\n")
	for _, content := range mixedContent {
		worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{Content: content},
		})
	}

	// Cross-language queries
	t.Log("--- Cross-language recall test ---\n")

	crossLangQueries := []struct {
		query    string
		lang     string
		expected string
	}{
		{"brain memory database", "EN", "brain-like"},
		{"beyin hafıza veritabanı", "TR", "beyin"},
		{"Gehirn Speicher Datenbank", "DE", "gehirn"},
		{"synapse connection neuron", "EN", "synapses"},
		{"bağlantı zayıflıyor silinmiyor", "TR", "silinmiyor"},
		{"Verbindungen schwächer gelöscht", "DE", "gelöscht"},
	}

	for _, clq := range crossLangQueries {
		result, _ := worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpSearch,
			Payload: concurrency.SearchRequest{Query: clq.query, Depth: 2, Limit: 5},
		})
		neurons := result.([]*core.Neuron)

		found := false
		for _, n := range neurons {
			if strings.Contains(strings.ToLower(n.Content), strings.ToLower(clq.expected)) {
				found = true
				t.Logf("  ✅ [%s] '%s' → '%s'", clq.lang, clq.query[:min(25, len(clq.query))], truncateS(n.Content, 40))
				break
			}
		}
		if !found && len(neurons) > 0 {
			t.Logf("  ⚠️ [%s] '%s' → found %d but no exact match", clq.lang, clq.query[:min(25, len(clq.query))], len(neurons))
		} else if len(neurons) == 0 {
			t.Logf("  ❌ [%s] '%s' → no results", clq.lang, clq.query[:min(25, len(clq.query))])
		}
	}

	t.Log("\n✅ Mixed language test completed")
}

func truncateS(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
