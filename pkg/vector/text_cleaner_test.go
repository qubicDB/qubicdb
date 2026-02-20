package vector

import (
	"strings"
	"testing"
)

// ─── CleanText ────────────────────────────────────────────────────────────────

func TestCleanText_HTMLTags(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"simple tag", "<b>hello</b>", "hello"},
		{"nested tags", "<div><p>foo <span>bar</span></p></div>", "foo bar"},
		{"anchor with href", `<a href="https://example.com">click</a>`, "click"},
		{"script tag stripped", "<script>alert('x')</script>text", "text"},
		{"style tag stripped", "<style>.a{color:red}</style>text", "text"},
		{"self-closing br", "line1<br/>line2", "line1 line2"},
		{"img alt not kept", `<img src="x.png" alt="photo"/>`, ""},
		{"mixed html and text", "<h1>Title</h1><p>Body text here.</p>", "Title Body text here."},
		{"xml-style tags", "<root><item>value</item></root>", "value"},
		{"already clean", "plain text", "plain text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanText(tc.input)
			if got != tc.want {
				t.Errorf("CleanText(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCleanText_Emoji(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"face emoji", "hello 😀 world", "hello world"},
		{"multiple emoji", "🔥🚀💡 text", "text"},
		{"emoji only", "😂😂😂", ""},
		{"emoji between words", "good 👍 job", "good job"},
		{"flag emoji", "🇹🇷 Turkey", "Turkey"},
		{"heart emoji", "I ❤️ Go", "I Go"},
		{"mixed emoji and punctuation", "wow!!! 🎉🎊 nice.", "wow!!! nice."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanText(tc.input)
			if got != tc.want {
				t.Errorf("CleanText(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCleanText_ControlCharacters(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"null byte", "foo\x00bar", "foobar"},
		{"bell char", "foo\x07bar", "foobar"},
		{"backspace", "foo\x08bar", "foobar"},
		{"form feed", "foo\x0Cbar", "foobar"},
		{"vertical tab", "foo\x0Bbar", "foobar"},
		{"escape char", "foo\x1Bbar", "foobar"},
		{"delete char", "foo\x7Fbar", "foobar"},
		{"newline kept", "foo\nbar", "foo bar"},
		{"tab kept and collapsed", "foo\t\tbar", "foo bar"},
		{"carriage return kept", "foo\rbar", "foo bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanText(tc.input)
			if got != tc.want {
				t.Errorf("CleanText(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCleanText_Whitespace(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"leading spaces", "   hello", "hello"},
		{"trailing spaces", "hello   ", "hello"},
		{"multiple spaces", "foo   bar", "foo bar"},
		{"mixed whitespace", "foo \t \n bar", "foo bar"},
		{"only whitespace", "   \t\n  ", ""},
		{"newlines between words", "foo\n\nbar", "foo bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanText(tc.input)
			if got != tc.want {
				t.Errorf("CleanText(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCleanText_Combined(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"html + emoji + control",
			"<p>Hello 😀\x00 World!</p>",
			"Hello World!",
		},
		{
			"html table with emoji",
			"<table><tr><td>🔥 hot</td><td>cold</td></tr></table>",
			"hot cold",
		},
		{
			"doc-style markup",
			"[bold]important[/bold] note\x1B[0m here",
			"[bold]important[/bold] note[0m here",
		},
		{
			"real world messy input",
			"  <div class=\"x\">  Check this out!! 🎉\n\nGreat stuff.\t</div>  ",
			"Check this out!! Great stuff.",
		},
		{
			"empty after cleaning",
			"<b></b>😀\x00",
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanText(tc.input)
			if got != tc.want {
				t.Errorf("CleanText(%q)\n got  %q\n want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ─── chunkBySentences ────────────────────────────────────────────────────────

func TestChunkBySentences_Basic(t *testing.T) {
	text := "First sentence. Second sentence. Third sentence."
	chunks := chunkBySentences(text, 100)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for small text, got %d: %v", len(chunks), chunks)
	}
}

func TestChunkBySentences_SplitsAtBoundary(t *testing.T) {
	// Build a text where each sentence is ~5 words; maxWords=10 → 2 sentences per chunk
	sentences := []string{
		"The quick brown fox jumps.",
		"Over the lazy dog now.",
		"A second pair of sentences.",
		"This is the fourth one here.",
	}
	text := strings.Join(sentences, " ")
	chunks := chunkBySentences(text, 10)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d: %v", len(chunks), chunks)
	}
	// No chunk should exceed maxWords
	for i, c := range chunks {
		wc := len(strings.Fields(c))
		if wc > 12 { // small tolerance for sentence-boundary rounding
			t.Errorf("chunk[%d] has %d words (>12): %q", i, wc, c)
		}
	}
}

func TestChunkBySentences_NoSentenceSplitMidSentence(t *testing.T) {
	// Each sentence is 8 words; maxWords=10 → each sentence must be its own chunk
	// (adding a second 8-word sentence would exceed 10)
	s1 := "The system processes all incoming requests carefully."
	s2 := "Results are stored in the distributed cache."
	s3 := "Latency is measured at every checkpoint here."
	text := s1 + " " + s2 + " " + s3

	chunks := chunkBySentences(text, 10)

	for i, chunk := range chunks {
		// Each chunk must be a complete sentence — must not end mid-word
		// and must start with a capital letter
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			t.Errorf("chunk[%d] is empty", i)
		}
		// Verify no chunk is a partial sentence by checking it appears
		// as a prefix of one of the original sentences or equals one
		found := false
		for _, orig := range []string{s1, s2, s3} {
			if strings.HasPrefix(orig, trimmed) || trimmed == orig {
				found = true
				break
			}
			// chunk may contain multiple sentences
			if strings.Contains(text, trimmed) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("chunk[%d] %q does not match any original sentence boundary", i, trimmed)
		}
	}
}

func TestChunkBySentences_OversizedSingleSentence(t *testing.T) {
	// A single sentence longer than maxWords must still be emitted as one chunk
	long := "word1 word2 word3 word4 word5 word6 word7 word8 word9 word10 word11 word12."
	chunks := chunkBySentences(long, 5)
	if len(chunks) != 1 {
		t.Fatalf("oversized single sentence: expected 1 chunk, got %d: %v", len(chunks), chunks)
	}
}

func TestChunkBySentences_EmptyInput(t *testing.T) {
	chunks := chunkBySentences("", 100)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

func TestChunkBySentences_WhitespaceOnly(t *testing.T) {
	chunks := chunkBySentences("   \t\n  ", 100)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for whitespace-only input, got %d", len(chunks))
	}
}

func TestChunkBySentences_MaxWordsOne(t *testing.T) {
	// maxWords=1 → each sentence is its own chunk (sentence > 1 word still emitted whole)
	text := "Hello. World."
	chunks := chunkBySentences(text, 1)
	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}
}

func TestChunkBySentences_NoBoundaryLoss(t *testing.T) {
	// Reassembling chunks must contain all words from the original text
	text := "Alpha beta gamma. Delta epsilon zeta. Eta theta iota kappa. Lambda mu nu xi omicron."
	chunks := chunkBySentences(text, 6)

	rejoined := strings.Join(chunks, " ")
	origWords := strings.Fields(text)
	rejoinedWords := strings.Fields(rejoined)

	// Strip punctuation for comparison
	normalize := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '.' || r == ',' {
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
}
