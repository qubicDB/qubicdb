package concurrency

import (
	"fmt"
	"testing"

	"github.com/denizumutdereli/qubicdb/pkg/core"
)

func BenchmarkBrainWorkerAddNeuron(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	w := NewBrainWorker("bench-user", m)
	defer w.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Submit(&Operation{
			Type: OpWrite,
			Payload: AddNeuronRequest{
				Content: fmt.Sprintf("Benchmark content %d", i),
			},
		})
	}
}

func BenchmarkBrainWorkerSearch(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	w := NewBrainWorker("bench-user", m)
	defer w.Stop()

	// Pre-populate
	for i := 0; i < 500; i++ {
		w.Submit(&Operation{
			Type: OpWrite,
			Payload: AddNeuronRequest{
				Content: fmt.Sprintf("Content about TypeScript programming %d", i),
			},
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Submit(&Operation{
			Type: OpSearch,
			Payload: SearchRequest{
				Query: "TypeScript",
				Depth: 1,
				Limit: 10,
			},
		})
	}
}

func BenchmarkBrainWorkerAsync(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	w := NewBrainWorker("bench-user", m)
	defer w.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.SubmitAsync(&Operation{
			Type: OpWrite,
			Payload: AddNeuronRequest{
				Content: fmt.Sprintf("Async content %d", i),
			},
		})
	}
}

func BenchmarkBrainWorkerParallel(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	w := NewBrainWorker("bench-user", m)
	defer w.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			w.SubmitAsync(&Operation{
				Type: OpWrite,
				Payload: AddNeuronRequest{
					Content: fmt.Sprintf("Parallel content %d", i),
				},
			})
			i++
		}
	})
}
