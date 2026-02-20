package simd

import (
	"math"
	"testing"
)

func TestGenericCosine_Identical(t *testing.T) {
	a := []float32{1, 2, 3, 4}
	b := []float32{1, 2, 3, 4}
	result := genericCosine(a, b)
	if math.Abs(result-1.0) > 1e-6 {
		t.Fatalf("expected ~1.0 for identical vectors, got %f", result)
	}
}

func TestGenericCosine_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	result := genericCosine(a, b)
	if math.Abs(result) > 1e-6 {
		t.Fatalf("expected ~0.0 for orthogonal vectors, got %f", result)
	}
}

func TestGenericCosine_Opposite(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{-1, 0, 0}
	result := genericCosine(a, b)
	if math.Abs(result+1.0) > 1e-6 {
		t.Fatalf("expected ~-1.0 for opposite vectors, got %f", result)
	}
}

func TestGenericCosine_Zero(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	result := genericCosine(a, b)
	if result != 0 {
		t.Fatalf("expected 0 for zero vector, got %f", result)
	}
}

func TestGenericDotProduct(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{4, 5, 6}
	result := genericDotProduct(a, b)
	expected := 1.0*4 + 2.0*5 + 3.0*6
	if math.Abs(result-expected) > 1e-6 {
		t.Fatalf("expected %f, got %f", expected, result)
	}
}

func TestCosine_MatchesGeneric(t *testing.T) {
	a := []float32{0.1, 0.5, -0.3, 0.8, 0.2, -0.1, 0.4, 0.6}
	b := []float32{0.3, -0.2, 0.7, 0.1, 0.5, 0.3, -0.4, 0.2}

	var simdResult float64
	Cosine(&simdResult, a, b)
	genericResult := genericCosine(a, b)

	if math.Abs(simdResult-genericResult) > 1e-4 {
		t.Fatalf("SIMD result %f differs from generic %f", simdResult, genericResult)
	}
}

func TestDotProduct_MatchesGeneric(t *testing.T) {
	a := []float32{0.1, 0.5, -0.3, 0.8}
	b := []float32{0.3, -0.2, 0.7, 0.1}

	var simdResult float64
	DotProduct(&simdResult, a, b)
	genericResult := genericDotProduct(a, b)

	if math.Abs(simdResult-genericResult) > 1e-4 {
		t.Fatalf("SIMD result %f differs from generic %f", simdResult, genericResult)
	}
}

func BenchmarkCosine_384(b *testing.B) {
	a := make([]float32, 384)
	c := make([]float32, 384)
	for i := range a {
		a[i] = float32(i) * 0.01
		c[i] = float32(i) * 0.02
	}
	var dst float64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Cosine(&dst, a, c)
	}
}
