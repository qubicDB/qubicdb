// SIMD-accelerated cosine similarity and dot product.
// Adapted from kelindar/search (MIT License).

package simd

import (
	"math"
	"runtime"
	"unsafe"

	"github.com/klauspost/cpuid/v2"
)

var (
	avx2     = cpuid.CPU.Supports(cpuid.AVX2) && cpuid.CPU.Supports(cpuid.FMA3)
	apple    = runtime.GOARCH == "arm64" && runtime.GOOS == "darwin"
	neon     = runtime.GOARCH == "arm64" && cpuid.CPU.Supports(cpuid.SVE)
	hardware = avx2 || apple || neon
)

// Cosine calculates the cosine similarity between two vectors.
func Cosine(dst *float64, a, b []float32) {
	if len(a) != len(b) {
		panic("vectors must be of same length")
	}
	if len(a) == 0 {
		*dst = 0
		return
	}

	switch {
	case hardware:
		if runSIMDCosine(dst, a, b) {
			return
		}
		*dst = genericCosine(a, b)
	default:
		*dst = genericCosine(a, b)
	}
}

// DotProduct calculates the dot product between two vectors.
func DotProduct(dst *float64, a, b []float32) {
	if len(a) != len(b) {
		panic("vectors must be of same length")
	}
	if len(a) == 0 {
		*dst = 0
		return
	}

	switch {
	case hardware:
		if runSIMDDotProduct(dst, a, b) {
			return
		}
		*dst = genericDotProduct(a, b)
	default:
		*dst = genericDotProduct(a, b)
	}
}

func runSIMDCosine(dst *float64, a, b []float32) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	f32_cosine_distance(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), unsafe.Pointer(dst), uint64(len(a)))
	return true
}

func runSIMDDotProduct(dst *float64, a, b []float32) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	f32_dot_product(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), unsafe.Pointer(dst), uint64(len(a)))
	return true
}

func genericCosine(x, y []float32) float64 {
	var sum_xy, sum_xx, sum_yy float64
	for i := range x {
		sum_xy += float64(x[i] * y[i])
		sum_xx += float64(x[i] * x[i])
		sum_yy += float64(y[i] * y[i])
	}

	denominator := math.Sqrt(sum_xx) * math.Sqrt(sum_yy)
	if denominator == 0 {
		return 0.0
	}

	return sum_xy / denominator
}

func genericDotProduct(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i] * b[i])
	}
	return sum
}
