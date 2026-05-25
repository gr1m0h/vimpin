package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// defaultDiscoverGlobs is the list of paths walked when the user invokes
// vimpin without explicit path arguments. The set matches the LazyVim
// convention plus the common single-file fallbacks; users with a different
// layout can pass paths explicitly.
var defaultDiscoverGlobs = []string{
	"lua/plugins",
	"lua/config/lazy.lua",
	"init.lua",
	"plugin",
}

// discoverFiles expands the given paths into a deduplicated, sorted list of
// .lua files. Directories are walked recursively. Files are returned even if
// they do not exist on disk; the caller surfaces the resulting error.
//
// When paths is empty the function falls back to defaultDiscoverGlobs. Any
// default path that does not exist is skipped silently so a project with only
// a lua/plugins directory still works without configuration.
func discoverFiles(paths []string) ([]string, error) {
	if len(paths) == 0 {
		paths = nil
		for _, p := range defaultDiscoverGlobs {
			if _, err := os.Stat(p); err == nil {
				paths = append(paths, p)
			}
		}
		if len(paths) == 0 {
			return nil, fmt.Errorf("no default lua source paths found (looked in %s)",
				strings.Join(defaultDiscoverGlobs, ", "))
		}
	}

	seen := make(map[string]struct{})
	var out []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}
		if info.IsDir() {
			err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				if !strings.HasSuffix(path, ".lua") {
					return nil
				}
				if _, ok := seen[path]; ok {
					return nil
				}
				seen[path] = struct{}{}
				out = append(out, path)
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("walk %s: %w", p, err)
			}
			continue
		}
		if !strings.HasSuffix(p, ".lua") {
			return nil, fmt.Errorf("%s is not a .lua file", p)
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}
