// Vectorizer wraps a GGUF embedding model for internal text → vector conversion.
// Adapted from kelindar/search (MIT License).

package vector

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/denizumutdereli/qubicdb/pkg/vector/simd"
	"github.com/sentencizer/sentencizer"
)

// Vectorizer represents a loaded GGUF embedding model.
type Vectorizer struct {
	handle  uintptr
	dim     int32
	ctxSize uint32
	pool    *ctxPool
}

// NewVectorizer loads a GGUF model file and returns a ready-to-use vectorizer.
// gpuLayers controls how many model layers are offloaded to GPU (0 = CPU only).
// embedContextSize sets the llama.cpp context window; 0 defaults to 512.
func NewVectorizer(modelPath string, gpuLayers int, embedContextSize uint32) (*Vectorizer, error) {
	if err := initLibrary(); err != nil {
		return nil, fmt.Errorf("failed to initialize llama library: %w", err)
	}

	handle := load_model(modelPath, uint32(gpuLayers))
	if handle == 0 {
		return nil, fmt.Errorf("failed to load model (%s)", modelPath)
	}

	ctxSize := embedContextSize
	if ctxSize < 512 {
		ctxSize = 512
	}

	v := &Vectorizer{
		handle:  handle,
		dim:     embed_size(handle),
		ctxSize: ctxSize,
	}
	v.pool = newCtxPool(16, func() *embedCtx {
		return v.newContext(ctxSize)
	})

	return v, nil
}

// EmbedText cleans the input text and converts it into a float32 embedding
// vector. If the text exceeds the model's token budget it is split into
// sentence-boundary-aware chunks and the embeddings are averaged.
func (v *Vectorizer) EmbedText(text string) ([]float32, error) {
	text = CleanText(text)
	if text == "" {
		return nil, fmt.Errorf("embed_text: text is empty after cleaning")
	}

	ctx := v.pool.get()
	defer v.pool.put(ctx)

	out := make([]float32, v.dim)
	var tokens uint32

	rc := embed_text(ctx.handle, text, out, &tokens)
	if rc == 0 {
		return out, nil
	}

	// rc=1: token count exceeds batch size — chunk and average.
	if rc == 1 {
		return v.embedChunked(text)
	}

	return nil, fmt.Errorf("embed_text failed (rc=%d, tokens=%d)", rc, tokens)
}

// embedChunked splits text into sentence-boundary-aware chunks of at most
// ctxSize/2 words and returns the averaged, normalized embedding.
func (v *Vectorizer) embedChunked(text string) ([]float32, error) {
	chunks := chunkBySentences(text, int(v.ctxSize)/2)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("embed_text: text produced no embeddable chunks")
	}

	sum := make([]float64, v.dim)
	var total int

	for _, chunk := range chunks {
		ctx := v.pool.get()
		out := make([]float32, v.dim)
		var tokens uint32
		rc := embed_text(ctx.handle, chunk, out, &tokens)
		v.pool.put(ctx)
		if rc != 0 {
			continue
		}
		for i, val := range out {
			sum[i] += float64(val)
		}
		total++
	}

	if total == 0 {
		return nil, fmt.Errorf("embed_text: all chunks failed to embed")
	}

	result := make([]float32, v.dim)
	for i := range result {
		result[i] = float32(sum[i] / float64(total))
	}
	Normalize(result)
	return result, nil
}

// segmenterEn is a package-level English sentence segmenter (thread-safe).
var segmenterEn = sentencizer.NewSegmenter("en")

// chunkBySentences groups sentences into chunks whose word count does not
// exceed maxWords. Sentence boundaries are never split mid-sentence.
// If a single sentence exceeds maxWords it is emitted as its own chunk.
func chunkBySentences(text string, maxWords int) []string {
	if maxWords < 1 {
		maxWords = 1
	}
	sentences := segmenterEn.Segment(text)
	if len(sentences) == 0 {
		return nil
	}

	var chunks []string
	var buf strings.Builder
	wordCount := 0

	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		wc := len(strings.Fields(s))
		if wordCount+wc > maxWords && buf.Len() > 0 {
			chunks = append(chunks, strings.TrimSpace(buf.String()))
			buf.Reset()
			wordCount = 0
		}
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(s)
		wordCount += wc
	}
	if buf.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(buf.String()))
	}
	return chunks
}

// ChunkBySentencesExported is the exported wrapper for chunkBySentences,
// used in integration and e2e tests.
func ChunkBySentencesExported(text string, maxWords int) []string {
	return chunkBySentences(text, maxWords)
}

// EmbedDim returns the dimensionality of the model's embedding vectors.
func (v *Vectorizer) EmbedDim() int {
	return int(v.dim)
}

// Close releases all resources held by the vectorizer.
func (v *Vectorizer) Close() error {
	if v.pool != nil {
		v.pool.close(func(ctx *embedCtx) {
			free_context(ctx.handle)
		})
	}
	if v.handle != 0 {
		free_model(v.handle)
		v.handle = 0
	}
	return nil
}

// CosineSimilarity computes the cosine similarity between two embedding vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var result float64
	simd.Cosine(&result, a, b)
	return result
}

// DotProduct computes the dot product between two normalized embedding vectors.
func DotProduct(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var result float64
	simd.DotProduct(&result, a, b)
	return result
}

// Normalize normalizes a vector in-place to unit length.
func Normalize(v []float32) {
	var sum float64
	for _, val := range v {
		sum += float64(val) * float64(val)
	}
	norm := float32(math.Sqrt(sum))
	if norm == 0 {
		return
	}
	for i := range v {
		v[i] /= norm
	}
}

// ---------------------------------- internal context pool ----------------------------------

type embedCtx struct {
	handle uintptr
}

func (v *Vectorizer) newContext(ctxSize uint32) *embedCtx {
	if ctxSize == 0 {
		ctxSize = 512
	}
	h := load_context(v.handle, ctxSize, true)
	return &embedCtx{handle: h}
}

type ctxPool struct {
	pool  chan *embedCtx
	newFn func() *embedCtx
	mu    sync.Mutex
}

func newCtxPool(size int, newFn func() *embedCtx) *ctxPool {
	p := &ctxPool{
		pool:  make(chan *embedCtx, size),
		newFn: newFn,
	}
	// Pre-create one context for immediate use
	p.pool <- newFn()
	return p
}

func (p *ctxPool) get() *embedCtx {
	select {
	case ctx := <-p.pool:
		return ctx
	default:
		return p.newFn()
	}
}

func (p *ctxPool) put(ctx *embedCtx) {
	select {
	case p.pool <- ctx:
	default:
	}
}

func (p *ctxPool) close(freeFn func(*embedCtx)) {
	close(p.pool)
	for ctx := range p.pool {
		freeFn(ctx)
	}
}
