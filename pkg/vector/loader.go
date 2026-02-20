// Dynamic library loader for llama.cpp via purego (no cgo).
// Adapted from kelindar/search (MIT License).

package vector

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

var (
	libptr       uintptr
	libOnce      sync.Once
	libErr       error
	load_library func(log_level int) uintptr
	load_model   func(path_model string, n_gpu_layers uint32) uintptr
	load_context func(model uintptr, ctx_size uint32, embeddings bool) uintptr
	free_model   func(model uintptr)
	free_context func(ctx uintptr)
	embed_size   func(model uintptr) int32
	embed_text   func(model uintptr, text string, out_embeddings []float32, out_tokens *uint32) int
)

// initLibrary lazily loads the llama.cpp shared library on first use.
func initLibrary() error {
	libOnce.Do(func() {
		libpath, err := findLlama()
		if err != nil {
			libErr = err
			return
		}
		if libptr, err = load(libpath); err != nil {
			libErr = err
			return
		}

		purego.RegisterLibFunc(&load_library, libptr, "load_library")
		purego.RegisterLibFunc(&load_model, libptr, "load_model")
		purego.RegisterLibFunc(&load_context, libptr, "load_context")
		purego.RegisterLibFunc(&free_model, libptr, "free_model")
		purego.RegisterLibFunc(&free_context, libptr, "free_context")
		purego.RegisterLibFunc(&embed_size, libptr, "embed_size")
		purego.RegisterLibFunc(&embed_text, libptr, "embed_text")

		load_library(2)
	})
	return libErr
}

// --------------------------------- Library Lookup ---------------------------------

func findLlama() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if path, err := findLibrary("llama_go.dll", runtime.GOOS); err == nil {
			return path, nil
		}
		return findLibrary("llamago.dll", runtime.GOOS)
	case "darwin":
		return findLibrary("libllama_go.dylib", runtime.GOOS)
	default:
		return findLibrary("libllama_go.so", runtime.GOOS)
	}
}

func findLibrary(name, goos string) (string, error) {
	dirs := libDirs(goos)
	checked := make([]string, 0, len(dirs))

	for _, dir := range dirs {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		checked = append(checked, path)
	}

	return "", fmt.Errorf("library '%s' not found, checked following paths:\n\t - %s",
		name, strings.Join(checked, "\n\t - "))
}

func libDirs(goos string) []string {
	dirs := []string{"/usr/lib", "/usr/local/lib"}

	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}

	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
		current := wd
		for i := 0; i < 3; i++ {
			parent := filepath.Dir(current)
			if parent == current || parent == "." || parent == "" {
				break
			}
			dirs = append(dirs, parent)
			current = parent
		}
	}

	switch goos {
	case "windows":
		if sys := os.Getenv("SYSTEMROOT"); sys != "" {
			dirs = append(dirs, filepath.Join(sys, "System32"))
		}
	case "darwin":
		dirs = append(dirs, "/opt/homebrew/lib")
	}

	for _, envKey := range []string{"LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH"} {
		if val := os.Getenv(envKey); val != "" {
			dirs = append(dirs, strings.Split(val, ":")...)
		}
	}

	if goos == "windows" {
		if val := os.Getenv("PATH"); val != "" {
			dirs = append(dirs, strings.Split(val, ";")...)
		}
	}

	return dirs
}

// ResolveLibraryError returns a user-friendly error message when library is not found.
func ResolveLibraryError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "not found") {
		return fmt.Sprintf("llama.cpp shared library not found.\n"+
			"Please build libllama_go and install it:\n"+
			"  macOS:   sudo cp libllama_go.dylib /usr/local/lib/\n"+
			"  Linux:   sudo cp libllama_go.so /usr/local/lib/ && sudo ldconfig\n"+
			"  Windows: copy llama_go.dll to the executable directory\n\n"+
			"Original error: %s", msg)
	}
	return msg
}

// --------------------------------- Exported Helpers ---------------------------------

// LibDirs returns the list of directories searched for the shared library.
// Exported for testing.
func LibDirs(goos string) []string {
	return libDirs(goos)
}

// FindLibrary is exported for testing.
func FindLibrary(name, goos string) (string, error) {
	return findLibrary(name, goos)
}

// IsLibraryAvailable checks if the llama.cpp library can be found without loading it.
func IsLibraryAvailable() bool {
	_, err := findLlama()
	return err == nil
}

// LibraryNotFoundError is a sentinel check.
func LibraryNotFoundError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

// ErrLibraryNotFound is returned when the shared library cannot be located.
var ErrLibraryNotFound = errors.New("llama.cpp shared library not found")
