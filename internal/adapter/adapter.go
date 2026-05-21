// Package adapter defines the interface that plugin-manager-specific code
// generators implement, and a small registry so the CLI can look them up by
// name without importing every adapter package directly.
package adapter

import (
	"fmt"
	"io"
	"sort"

	"github.com/gr1m0h/vimpin/internal/manifest"
)

// Adapter renders a manifest into a plugin-manager-specific spec.
type Adapter interface {
	// Name returns the identifier used on the CLI (e.g. "lazy").
	Name() string
	// Generate writes the rendered spec to w.
	Generate(m *manifest.Manifest, plugins []manifest.Plugin, w io.Writer) error
}

var registry = map[string]Adapter{}

// Register makes an adapter available under its Name().
// Adapters are expected to call Register from an init() function.
func Register(a Adapter) {
	if _, ok := registry[a.Name()]; ok {
		panic(fmt.Sprintf("adapter %q already registered", a.Name()))
	}
	registry[a.Name()] = a
}

// Get returns the adapter registered under name.
func Get(name string) (Adapter, error) {
	a, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown adapter %q (available: %v)", name, Names())
	}
	return a, nil
}

// Names returns the sorted list of registered adapter names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// FilterByGroups returns the subset of plugins whose group is in keep.
// When keep is empty, every plugin is returned. A plugin with no group is
// treated as belonging to "default".
func FilterByGroups(plugins []manifest.Plugin, keep []string) []manifest.Plugin {
	if len(keep) == 0 {
		return plugins
	}
	allow := make(map[string]bool, len(keep))
	for _, g := range keep {
		allow[g] = true
	}
	out := make([]manifest.Plugin, 0, len(plugins))
	for _, p := range plugins {
		g := p.Group
		if g == "" {
			g = "default"
		}
		if allow[g] {
			out = append(out, p)
		}
	}
	return out
}
