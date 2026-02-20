//go:build !windows

package vector

import "github.com/ebitengine/purego"

func load(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}
