package vector

import (
	"runtime"
	"testing"
)

func TestLibDirs_ContainsStandardPaths(t *testing.T) {
	dirs := LibDirs(runtime.GOOS)
	if len(dirs) == 0 {
		t.Fatal("expected at least one library directory")
	}

	// Should contain /usr/lib or /usr/local/lib on unix
	if runtime.GOOS != "windows" {
		found := false
		for _, d := range dirs {
			if d == "/usr/lib" || d == "/usr/local/lib" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected standard unix lib dirs, got: %v", dirs)
		}
	}
}

func TestLibDirs_Darwin_HasHomebrew(t *testing.T) {
	dirs := LibDirs("darwin")
	found := false
	for _, d := range dirs {
		if d == "/opt/homebrew/lib" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected /opt/homebrew/lib in darwin lib dirs")
	}
}

func TestFindLibrary_NotFound(t *testing.T) {
	_, err := FindLibrary("nonexistent_lib_12345.so", runtime.GOOS)
	if err == nil {
		t.Fatal("expected error for nonexistent library")
	}
	if !LibraryNotFoundError(err) {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float32{1, 0, 0, 0}
	b := []float32{1, 0, 0, 0}
	sim := CosineSimilarity(a, b)
	if sim < 0.99 {
		t.Fatalf("expected ~1.0 for identical vectors, got %f", sim)
	}
}

func TestCosineSimilarity_EmptyVectors(t *testing.T) {
	sim := CosineSimilarity(nil, nil)
	if sim != 0 {
		t.Fatalf("expected 0 for nil vectors, got %f", sim)
	}
	sim = CosineSimilarity([]float32{1}, []float32{1, 2})
	if sim != 0 {
		t.Fatalf("expected 0 for mismatched lengths, got %f", sim)
	}
}

func TestNormalize(t *testing.T) {
	v := []float32{3, 4}
	Normalize(v)
	// Should be unit length: 3/5, 4/5
	if v[0] < 0.59 || v[0] > 0.61 {
		t.Fatalf("expected ~0.6 for v[0], got %f", v[0])
	}
	if v[1] < 0.79 || v[1] > 0.81 {
		t.Fatalf("expected ~0.8 for v[1], got %f", v[1])
	}
}

func TestNormalize_ZeroVector(t *testing.T) {
	v := []float32{0, 0, 0}
	Normalize(v) // should not panic
	for _, val := range v {
		if val != 0 {
			t.Fatalf("expected all zeros, got %f", val)
		}
	}
}
