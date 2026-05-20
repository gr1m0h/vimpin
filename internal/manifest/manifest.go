// Package manifest parses, validates, and writes vimpin manifest files.
//
// A vimpin manifest is a TOML file that lists Vim/Neovim plugins and their
// pinned versions. The commit field is the actual lock; tag and branch are
// hints for how Renovate should track updates.
package manifest

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const SchemaV1 = "https://vimpin.io/schema/v1"

// Manifest is the top-level structure of a vimpin manifest file.
type Manifest struct {
	Schema   string   `toml:"schema"`
	Settings Settings `toml:"settings"`
	Plugins  []Plugin `toml:"plugin"`
}

// Settings holds manifest-wide configuration.
type Settings struct {
	DefaultHost string   `toml:"default_host"`
	AllowHosts  []string `toml:"allow_hosts"`
}

// Plugin is a single plugin entry. Commit is the actual pin; Tag and Branch
// describe how Renovate should track this entry for updates.
//
// Resolution priority (commit > tag > branch):
//   - commit present: this is the version to install.
//   - commit empty, tag present: vimpin pin will resolve the tag to a commit.
//   - commit empty, branch present: vimpin pin will resolve the branch HEAD.
type Plugin struct {
	Repo   string `toml:"repo"`
	Commit string `toml:"commit,omitempty"`
	Tag    string `toml:"tag,omitempty"`
	Branch string `toml:"branch,omitempty"`
	Host   string `toml:"host,omitempty"`
	Layer  string `toml:"layer,omitempty"`
	Group  string `toml:"group,omitempty"`
	Reason string `toml:"reason,omitempty"`
}

// EffectiveHost returns the host to use for this plugin, falling back to the
// manifest-wide default if not set on the plugin itself.
func (p Plugin) EffectiveHost(s Settings) string {
	if p.Host != "" {
		return p.Host
	}
	if s.DefaultHost != "" {
		return s.DefaultHost
	}
	return "github.com"
}

// CloneURL returns the full URL used to fetch this plugin.
func (p Plugin) CloneURL(s Settings) string {
	return fmt.Sprintf("https://%s/%s.git", p.EffectiveHost(s), p.Repo)
}

// Load reads and parses a manifest file from disk.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return Parse(data)
}

// Parse decodes manifest bytes into a Manifest.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}
