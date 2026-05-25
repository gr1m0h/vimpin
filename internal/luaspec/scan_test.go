package luaspec

import (
	"strings"
	"testing"
)

func hex40(c byte) string { return strings.Repeat(string(c), 40) }

func TestScan_singleLineSpec(t *testing.T) {
	src := []byte(`return {
  { "ggandor/leap.nvim", tag = "v0.1.5" },
  { "folke/which-key.nvim", branch = "main" },
}
`)
	specs, err := Scan("plugins.lua", src)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("len(specs) = %d, want 2", len(specs))
	}
	if specs[0].Repo != "ggandor/leap.nvim" {
		t.Errorf("specs[0].Repo = %q", specs[0].Repo)
	}
	if specs[0].Tag() != "v0.1.5" {
		t.Errorf("specs[0].Tag() = %q", specs[0].Tag())
	}
	if specs[0].MultiLine {
		t.Errorf("specs[0].MultiLine = true, want false")
	}
	if specs[1].Branch() != "main" {
		t.Errorf("specs[1].Branch() = %q", specs[1].Branch())
	}
}

func TestScan_multiLineWithNestedTable(t *testing.T) {
	src := []byte(`return {
  {
    "ggandor/leap.nvim",
    tag = "v0.1.5",
    keys = { "s", "S" },
    config = function() end,
  },
}
`)
	specs, err := Scan("plugins.lua", src)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want 1", len(specs))
	}
	if !specs[0].MultiLine {
		t.Errorf("expected MultiLine=true")
	}
	if specs[0].Tag() != "v0.1.5" {
		t.Errorf("Tag = %q", specs[0].Tag())
	}
}

func TestScan_canonicalCommentForms(t *testing.T) {
	src := []byte(`return {
  { "ggandor/leap.nvim", commit = "` + hex40('a') + `" }, -- tag: v0.1.5
  {
    "folke/which-key.nvim",
    commit = "` + hex40('b') + `", -- branch: main
    event = "VeryLazy",
  },
}
`)
	specs, err := Scan("plugins.lua", src)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("len(specs) = %d, want 2", len(specs))
	}
	if specs[0].CommentRefType != RefTag || specs[0].CommentRef != "v0.1.5" {
		t.Errorf("specs[0] annotation = %v %q", specs[0].CommentRefType, specs[0].CommentRef)
	}
	if specs[1].CommentRefType != RefBranch || specs[1].CommentRef != "main" {
		t.Errorf("specs[1] annotation = %v %q", specs[1].CommentRefType, specs[1].CommentRef)
	}
}

func TestScan_ignoreMarker(t *testing.T) {
	src := []byte(`return {
  { "ggandor/leap.nvim", tag = "v0.1.5" }, -- vimpin:ignore
}
`)
	specs, err := Scan("plugins.lua", src)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d", len(specs))
	}
	if !specs[0].Ignored {
		t.Errorf("expected Ignored=true")
	}
}

func TestScan_skipsNonSpecTables(t *testing.T) {
	src := []byte(`local opts = { foo = "bar", baz = "qux" }
local keys = { "s", "S" }
return { { "owner/repo", tag = "v1.0" } }
`)
	specs, err := Scan("plugins.lua", src)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want 1 (only the owner/repo spec)", len(specs))
	}
}

func TestSpec_SourceRef_precedence(t *testing.T) {
	specs, err := Scan("x.lua", []byte(`return {
  { "a/b", tag = "v1.0", branch = "main" },
  { "c/d", commit = "`+hex40('c')+`" }, -- tag: v2.0
}
`))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	rt, rv := specs[0].SourceRef()
	if rt != RefTag || rv != "v1.0" {
		t.Errorf("specs[0].SourceRef = %v %q, want tag v1.0", rt, rv)
	}
	rt, rv = specs[1].SourceRef()
	if rt != RefTag || rv != "v2.0" {
		t.Errorf("specs[1].SourceRef = %v %q, want tag v2.0", rt, rv)
	}
}
