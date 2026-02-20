package core

import (
	"fmt"
	"strings"
	"sync/atomic"
)

const (
	// DefaultMaxNeuronContentBytes defines the default hard upper boundary for neuron text payloads.
	// Larger contents should be chunked by upstream clients/wrappers.
	DefaultMaxNeuronContentBytes = 64 * 1024

	// MaxNeuronContentBytes is retained as a compatibility alias for tests and callers.
	MaxNeuronContentBytes = DefaultMaxNeuronContentBytes
)

var maxNeuronContentBytes atomic.Int64

func init() {
	maxNeuronContentBytes.Store(DefaultMaxNeuronContentBytes)
}

// SetMaxNeuronContentBytes overrides the runtime neuron content size limit.
func SetMaxNeuronContentBytes(limit int64) error {
	if limit <= 0 {
		return fmt.Errorf("max neuron content bytes must be > 0")
	}
	maxNeuronContentBytes.Store(limit)
	return nil
}

// GetMaxNeuronContentBytes returns the active runtime neuron content size limit.
func GetMaxNeuronContentBytes() int64 {
	limit := maxNeuronContentBytes.Load()
	if limit <= 0 {
		return DefaultMaxNeuronContentBytes
	}
	return limit
}

// ValidateNeuronContent ensures neuron content is non-empty and within size boundaries.
func ValidateNeuronContent(content string) error {
	if strings.TrimSpace(content) == "" {
		return ErrInvalidContent
	}

	size := len([]byte(content))
	maxBytes := GetMaxNeuronContentBytes()
	if int64(size) > maxBytes {
		return fmt.Errorf("%w: %d bytes > %d", ErrContentTooLarge, size, maxBytes)
	}

	return nil
}
