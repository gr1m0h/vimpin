package manifest

import (
	"fmt"
	"strings"
	"testing"
)

// makeHash returns a 40-character all-c hex string for use as a fake commit.
func makeHash(c byte) string {
	return strings.Repeat(string(c), 40)
}

func makeManifest(extras string) string {
	return fmt.Sprintf(`schema = "https://vimpin.io/schema/v1"

[settings]
default_host = "github.com"
allow_hosts = ["github.com"]

[[plugin]]
repo = "ggandor/leap.nvim"
commit = "%s"
tag = "v0.1.5"
layer = "user"
group = "core"

[[plugin]]
repo = "nvim-treesitter/nvim-treesitter"
commit = "%s"
branch = "master"
layer = "override"
reason = "CVE-2024-XXXX"
group = "core"
%s`, makeHash('a'), makeHash('b'), extras)
}

func TestParseValidManifest(t *testing.T) {
	m, err := Parse([]byte(makeManifest("")))
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}
	if got, want := m.Schema, SchemaV1; got != want {
		t.Errorf("Schema = %q, want %q", got, want)
	}
	if got, want := len(m.Plugins), 2; got != want {
		t.Fatalf("len(Plugins) = %d, want %d", got, want)
	}
	if got, want := m.Plugins[0].Repo, "ggandor/leap.nvim"; got != want {
		t.Errorf("Plugins[0].Repo = %q, want %q", got, want)
	}
	if got, want := m.Plugins[1].Layer, "override"; got != want {
		t.Errorf("Plugins[1].Layer = %q, want %q", got, want)
	}
}

func TestValidateAcceptsValidManifest(t *testing.T) {
	m, err := Parse([]byte(makeManifest("")))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	hashA := makeHash('a')
	hashB := makeHash('b')

	cases := map[string]struct {
		manifest string
		want     string
	}{
		"missing schema": {
			manifest: fmt.Sprintf(`[[plugin]]
repo = "a/b"
commit = "%s"
`, hashA),
			want: "missing top-level schema",
		},
		"short commit": {
			manifest: `schema = "https://vimpin.io/schema/v1"
[settings]
allow_hosts = ["github.com"]
[[plugin]]
repo = "a/b"
commit = "abc123"
`,
			want: "not a 40-character lowercase hex",
		},
		"repo without slash": {
			manifest: fmt.Sprintf(`schema = "https://vimpin.io/schema/v1"
[settings]
allow_hosts = ["github.com"]
[[plugin]]
repo = "lone-name"
commit = "%s"
`, hashA),
			want: "owner/name form",
		},
		"disallowed host": {
			manifest: fmt.Sprintf(`schema = "https://vimpin.io/schema/v1"
[settings]
allow_hosts = ["github.com"]
[[plugin]]
repo = "a/b"
host = "evil.example.com"
commit = "%s"
`, hashA),
			want: "not in settings.allow_hosts",
		},
		"no ref at all": {
			manifest: `schema = "https://vimpin.io/schema/v1"
[settings]
allow_hosts = ["github.com"]
[[plugin]]
repo = "a/b"
`,
			want: "must have at least one of commit, tag, branch",
		},
		"invalid layer": {
			manifest: fmt.Sprintf(`schema = "https://vimpin.io/schema/v1"
[settings]
allow_hosts = ["github.com"]
[[plugin]]
repo = "a/b"
commit = "%s"
layer = "weird"
`, hashA),
			want: `layer must be "user" or "override"`,
		},
		"duplicate repo": {
			manifest: fmt.Sprintf(`schema = "https://vimpin.io/schema/v1"
[settings]
allow_hosts = ["github.com"]
[[plugin]]
repo = "a/b"
commit = "%s"
[[plugin]]
repo = "a/b"
commit = "%s"
`, hashA, hashB),
			want: "duplicate repo",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m, err := Parse([]byte(tc.manifest))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			err = m.Validate()
			if err == nil {
				t.Fatal("Validate: expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestEffectiveHost(t *testing.T) {
	s := Settings{DefaultHost: "github.com"}
	cases := []struct {
		name   string
		plugin Plugin
		want   string
	}{
		{"empty falls back to default", Plugin{}, "github.com"},
		{"explicit host wins", Plugin{Host: "git.sr.ht"}, "git.sr.ht"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.plugin.EffectiveHost(s); got != tc.want {
				t.Errorf("EffectiveHost = %q, want %q", got, tc.want)
			}
		})
	}
}
