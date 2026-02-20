package e2e

import (
	"strings"
	"testing"

	"github.com/denizumutdereli/qubicdb/pkg/vector"
)

// TestCleanTextPipeline_E2E verifies that CleanText produces non-empty,
// well-formed output for a variety of real-world dirty inputs without
// requiring the llama library.
func TestCleanTextPipeline_E2E(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		mustContain []string
		mustAbsent  []string
	}{
		{
			name:        "HTML document",
			input:       `<html><head><title>Test</title></head><body><h1>Distributed Systems</h1><p>Consensus algorithms ensure <b>fault tolerance</b> in clusters.</p></body></html>`,
			mustContain: []string{"Distributed Systems", "Consensus algorithms", "fault tolerance", "clusters"},
			mustAbsent:  []string{"<html>", "<head>", "<body>", "<h1>", "<p>", "<b>"},
		},
		{
			name:        "Markdown-style with emoji",
			input:       "## Overview 🚀\n\nThis system handles **vector search** and semantic retrieval. 🔍\n\n- Fast indexing\n- Low latency",
			mustContain: []string{"Overview", "vector search", "semantic retrieval", "Fast indexing", "Low latency"},
			mustAbsent:  []string{"🚀", "🔍"},
		},
		{
			name:        "XML data feed",
			input:       `<feed><entry><title>Neural Memory</title><content>Adaptive recall using hebbian linkage and depth decay.</content></entry></feed>`,
			mustContain: []string{"Neural Memory", "Adaptive recall", "hebbian linkage", "depth decay"},
			mustAbsent:  []string{"<feed>", "<entry>", "<title>", "<content>"},
		},
		{
			name:        "Control characters in text",
			input:       "Vector\x00search\x07is\x1Bfast\x7Fand\x08reliable.",
			mustContain: []string{"Vector", "search", "fast", "reliable"},
			mustAbsent:  []string{"\x00", "\x07", "\x1B", "\x7F", "\x08"},
		},
		{
			name:        "Mixed emoji and technical text",
			input:       "🔥 High throughput 💡 Low latency ⚡ Distributed consensus 🌐 Global replication",
			mustContain: []string{"High throughput", "Low latency", "Distributed consensus", "Global replication"},
			mustAbsent:  []string{"🔥", "💡", "⚡", "🌐"},
		},
		{
			name:        "Excessive whitespace",
			input:       "   Semantic   \t\t search   \n\n\n  engine   ",
			mustContain: []string{"Semantic search engine"},
			mustAbsent:  []string{"   ", "\t\t", "\n\n"},
		},
		{
			name:        "Already clean text unchanged",
			input:       "The quick brown fox jumps over the lazy dog.",
			mustContain: []string{"The quick brown fox jumps over the lazy dog."},
			mustAbsent:  []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := vector.CleanText(tc.input)

			for _, want := range tc.mustContain {
				if !strings.Contains(got, want) {
					t.Errorf("CleanText output missing %q\n  input: %q\n  got:   %q", want, tc.input, got)
				}
			}
			for _, absent := range tc.mustAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("CleanText output still contains %q\n  input: %q\n  got:   %q", absent, tc.input, got)
				}
			}
		})
	}
}

// TestChunkBySentences_E2E verifies sentence-boundary chunking on realistic
// multi-sentence paragraphs without requiring the llama library.
func TestChunkBySentences_E2E(t *testing.T) {
	paragraph := `Distributed systems require careful coordination between nodes. ` +
		`Consensus algorithms such as Raft and Paxos ensure that all replicas agree on a single value. ` +
		`Leader election is a critical step in this process. ` +
		`When a leader fails, a new election must complete before writes can resume. ` +
		`State machine replication guarantees that every node applies the same sequence of operations. ` +
		`This property is essential for building fault-tolerant databases and coordination services. ` +
		`Quorum-based approaches trade availability for consistency under network partitions. ` +
		`Modern systems often combine multiple strategies to balance these trade-offs effectively.`

	t.Run("chunks cover all words", func(t *testing.T) {
		chunks := vector.ChunkBySentencesExported(paragraph, 30)
		if len(chunks) == 0 {
			t.Fatal("expected at least one chunk")
		}

		rejoined := strings.Join(chunks, " ")
		origWords := strings.Fields(paragraph)
		rejoinedWords := strings.Fields(rejoined)

		normalize := func(s string) string {
			return strings.Map(func(r rune) rune {
				if r == '.' || r == ',' || r == ';' {
					return -1
				}
				return r
			}, strings.ToLower(s))
		}

		origSet := make(map[string]int)
		for _, w := range origWords {
			origSet[normalize(w)]++
		}
		rejoinedSet := make(map[string]int)
		for _, w := range rejoinedWords {
			rejoinedSet[normalize(w)]++
		}
		for word, count := range origSet {
			if rejoinedSet[word] < count {
				t.Errorf("word %q lost after chunking (orig=%d, rejoined=%d)", word, count, rejoinedSet[word])
			}
		}
	})

	t.Run("no chunk exceeds maxWords by more than one sentence", func(t *testing.T) {
		maxWords := 25
		chunks := vector.ChunkBySentencesExported(paragraph, maxWords)
		for i, c := range chunks {
			wc := len(strings.Fields(c))
			// A single oversized sentence may exceed maxWords — that is correct behaviour.
			// But a chunk must never contain more words than maxWords + the longest sentence.
			if wc > maxWords*2 {
				t.Errorf("chunk[%d] has %d words, far exceeds maxWords=%d: %q", i, wc, maxWords, c)
			}
		}
	})

	t.Run("each chunk is non-empty and trimmed", func(t *testing.T) {
		chunks := vector.ChunkBySentencesExported(paragraph, 20)
		for i, c := range chunks {
			if strings.TrimSpace(c) == "" {
				t.Errorf("chunk[%d] is empty or whitespace-only", i)
			}
			if c != strings.TrimSpace(c) {
				t.Errorf("chunk[%d] has leading/trailing whitespace: %q", i, c)
			}
		}
	})
}

// TestEmbedTextCleaningAndChunking_LiveModel runs against the real vectorizer
// when the library and model are available. Verifies that:
//   - Dirty HTML input is cleaned and embedded without error
//   - Long text (>512 tokens) is chunked and embedded without rc=1 error
//   - Empty-after-cleaning input returns an error, not a zero vector
func TestEmbedTextCleaningAndChunking_LiveModel(t *testing.T) {
	v := openLiveVectorizerOrSkip(t)
	defer v.Close()

	t.Run("dirty HTML input embeds successfully", func(t *testing.T) {
		dirty := `<html><body><h1>Distributed Systems 🔥</h1>` +
			`<p>Consensus algorithms ensure <b>fault tolerance</b> in clusters.</p>` +
			`<script>alert('xss')</script></body></html>`
		emb, err := v.EmbedText(dirty)
		if err != nil {
			t.Fatalf("EmbedText on dirty HTML failed: %v", err)
		}
		if len(emb) == 0 {
			t.Fatal("embedding is empty")
		}
		var nonZero int
		for _, f := range emb {
			if f != 0 {
				nonZero++
			}
		}
		if nonZero == 0 {
			t.Fatal("all embedding values are zero")
		}
	})

	t.Run("long text is chunked and embedded without error", func(t *testing.T) {
		// Build a text that is guaranteed to exceed 512 tokens (~400+ words)
		sentence := "Distributed consensus algorithms ensure fault tolerance in replicated state machines. "
		var sb strings.Builder
		for i := 0; i < 60; i++ {
			sb.WriteString(sentence)
		}
		long := sb.String()

		emb, err := v.EmbedText(long)
		if err != nil {
			t.Fatalf("EmbedText on long text failed: %v", err)
		}
		if len(emb) == 0 {
			t.Fatal("embedding is empty")
		}
	})

	t.Run("empty-after-cleaning returns error", func(t *testing.T) {
		_, err := v.EmbedText("<b></b>😀\x00\x07")
		if err == nil {
			t.Fatal("expected error for empty-after-cleaning input, got nil")
		}
	})

	t.Run("cosine similarity of same text is near 1.0", func(t *testing.T) {
		text := "Semantic search using vector embeddings for retrieval."
		a, err := v.EmbedText(text)
		if err != nil {
			t.Fatalf("first EmbedText failed: %v", err)
		}
		b, err := v.EmbedText(text)
		if err != nil {
			t.Fatalf("second EmbedText failed: %v", err)
		}
		sim := vector.CosineSimilarity(a, b)
		if sim < 0.99 {
			t.Errorf("cosine similarity of identical text = %.4f, want >= 0.99", sim)
		}
	})

	t.Run("similar texts have higher similarity than unrelated texts", func(t *testing.T) {
		textA := "Distributed consensus and fault tolerance in replicated systems."
		textB := "Raft and Paxos are consensus algorithms for distributed databases."
		textC := "Chocolate cake recipe with vanilla frosting and sprinkles."

		embA, err := v.EmbedText(textA)
		if err != nil {
			t.Fatalf("EmbedText A failed: %v", err)
		}
		embB, err := v.EmbedText(textB)
		if err != nil {
			t.Fatalf("EmbedText B failed: %v", err)
		}
		embC, err := v.EmbedText(textC)
		if err != nil {
			t.Fatalf("EmbedText C failed: %v", err)
		}

		simAB := vector.CosineSimilarity(embA, embB)
		simAC := vector.CosineSimilarity(embA, embC)

		if simAB <= simAC {
			t.Errorf("expected sim(A,B)=%.4f > sim(A,C)=%.4f but got the opposite", simAB, simAC)
		}
	})
}
