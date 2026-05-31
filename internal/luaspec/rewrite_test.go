package luaspec

import (
	"strings"
	"testing"
)

func TestApply_singleLineMigration(t *testing.T) {
	sha := strings.Repeat("a", 40)
	src := []byte(`return {
  { "ggandor/leap.nvim", tag = "v0.1.5" },
}
`)
	specs := mustScan(t, src)
	out, err := Apply(src, []Update{{
		Spec:      specs[0],
		NewCommit: sha,
		RefType:   RefTag,
		RefValue:  "v0.1.5",
	}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := `return {
  { "ggandor/leap.nvim", commit = "` + sha + `" }, -- tag: v0.1.5
}
`
	if string(out) != want {
		t.Errorf("mismatch:\n--- got\n%s\n--- want\n%s", string(out), want)
	}
}

func TestApply_multiLineMigrationWithConfig(t *testing.T) {
	sha := strings.Repeat("a", 40)
	src := []byte(`return {
  {
    "ggandor/leap.nvim",
    tag = "v0.1.5",
    keys = { "s", "S" },
    config = function() end,
  },
}
`)
	specs := mustScan(t, src)
	out, err := Apply(src, []Update{{
		Spec:      specs[0],
		NewCommit: sha,
		RefType:   RefTag,
		RefValue:  "v0.1.5",
	}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := `return {
  {
    "ggandor/leap.nvim",
    commit = "` + sha + `", -- tag: v0.1.5
    keys = { "s", "S" },
    config = function() end,
  },
}
`
	if string(out) != want {
		t.Errorf("mismatch:\n--- got\n%s\n--- want\n%s", string(out), want)
	}
}

func TestApply_branchAnnotation(t *testing.T) {
	sha := strings.Repeat("a", 40)
	src := []byte(`{ "folke/which-key.nvim", branch = "main" }
`)
	specs := mustScan(t, src)
	out, err := Apply(src, []Update{{
		Spec:      specs[0],
		NewCommit: sha,
		RefType:   RefBranch,
		RefValue:  "main",
	}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := `{ "folke/which-key.nvim", commit = "` + sha + `" } -- branch: main
`
	if string(out) != want {
		t.Errorf("mismatch:\n--- got\n%s\n--- want\n%s", string(out), want)
	}
}

func TestApply_refreshExistingCanonical(t *testing.T) {
	oldSha := strings.Repeat("b", 40)
	sha := strings.Repeat("a", 40)
	src := []byte(`return {
  { "a/b", commit = "` + oldSha + `" }, -- tag: v1.0
}
`)
	specs := mustScan(t, src)
	out, err := Apply(src, []Update{{
		Spec:      specs[0],
		NewCommit: sha,
		RefType:   RefTag,
		RefValue:  "v1.0",
	}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := `return {
  { "a/b", commit = "` + sha + `" }, -- tag: v1.0
}
`
	if string(out) != want {
		t.Errorf("mismatch:\n--- got\n%s\n--- want\n%s", string(out), want)
	}
}

func TestApply_removeStaleFieldFormTag(t *testing.T) {
	oldSha := strings.Repeat("c", 40)
	sha := strings.Repeat("a", 40)
	src := []byte(`return {
  { "a/b", commit = "` + oldSha + `", tag = "v1.0" },
}
`)
	specs := mustScan(t, src)
	out, err := Apply(src, []Update{{
		Spec:      specs[0],
		NewCommit: sha,
		RefType:   RefTag,
		RefValue:  "v1.0",
	}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := `return {
  { "a/b", commit = "` + sha + `" }, -- tag: v1.0
}
`
	if string(out) != want {
		t.Errorf("mismatch:\n--- got\n%s\n--- want\n%s", string(out), want)
	}
}

func TestApply_multipleSpecsInFile(t *testing.T) {
	sha := strings.Repeat("a", 40)
	src := []byte(`return {
  { "a/b", tag = "v1.0" },
  { "c/d", branch = "main" },
}
`)
	specs := mustScan(t, src)
	out, err := Apply(src, []Update{
		{Spec: specs[0], NewCommit: sha, RefType: RefTag, RefValue: "v1.0"},
		{Spec: specs[1], NewCommit: sha, RefType: RefBranch, RefValue: "main"},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := `return {
  { "a/b", commit = "` + sha + `" }, -- tag: v1.0
  { "c/d", commit = "` + sha + `" }, -- branch: main
}
`
	if string(out) != want {
		t.Errorf("mismatch:\n--- got\n%s\n--- want\n%s", string(out), want)
	}
}

func TestApply_errorsOnNoSourceRef(t *testing.T) {
	sha := strings.Repeat("a", 40)
	src := []byte(`{ "a/b" }
`)
	specs := mustScan(t, src)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	_, err := Apply(src, []Update{{
		Spec:      specs[0],
		NewCommit: sha,
		RefType:   RefTag,
		RefValue:  "v1.0",
	}})
	if err == nil {
		t.Fatal("expected error for spec with no source ref")
	}
}

func TestApply_commentOnly_replacesExistingAnnotation(t *testing.T) {
	sha := strings.Repeat("a", 40)
	src := []byte(`return {
  { "a/b", commit = "` + sha + `" }, -- tag: v1.0
}
`)
	specs := mustScan(t, src)
	out, err := Apply(src, []Update{{
		Spec:        specs[0],
		RefType:     RefTag,
		RefValue:    "v2.0",
		CommentOnly: true,
	}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := `return {
  { "a/b", commit = "` + sha + `" }, -- tag: v2.0
}
`
	if string(out) != want {
		t.Errorf("commit must not change, only annotation:\n--- got\n%s\n--- want\n%s", string(out), want)
	}
}

func TestApply_commentOnly_multiLineFormB(t *testing.T) {
	sha := strings.Repeat("a", 40)
	src := []byte(`return {
  {
    "a/b",
    commit = "` + sha + `", -- tag: v1.0
    keys = { "x" },
  },
}
`)
	specs := mustScan(t, src)
	out, err := Apply(src, []Update{{
		Spec:        specs[0],
		RefType:     RefTag,
		RefValue:    "v3.7.0",
		CommentOnly: true,
	}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(string(out), "-- tag: v3.7.0") {
		t.Errorf("annotation not updated:\n%s", string(out))
	}
	if !strings.Contains(string(out), `commit = "`+sha+`"`) {
		t.Errorf("commit should be unchanged:\n%s", string(out))
	}
	if !strings.Contains(string(out), `keys = { "x" }`) {
		t.Errorf("sibling field must remain:\n%s", string(out))
	}
}

func TestApply_commentOnly_errorsWithoutCommitField(t *testing.T) {
	src := []byte(`{ "a/b", tag = "v1.0" }
`)
	specs := mustScan(t, src)
	_, err := Apply(src, []Update{{
		Spec:        specs[0],
		RefType:     RefTag,
		RefValue:    "v2.0",
		CommentOnly: true,
	}})
	if err == nil {
		t.Fatal("expected error: --verify on a field-form spec has no SHA to anchor on")
	}
}

func mustScan(t *testing.T, src []byte) []Spec {
	t.Helper()
	specs, err := Scan("plugins.lua", src)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return specs
}
