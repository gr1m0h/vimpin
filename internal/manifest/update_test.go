package manifest

import (
	"strings"
	"testing"
)

func TestUpdateCommitsReplacesExisting(t *testing.T) {
	hashOld := makeHash('a')
	hashNew := makeHash('c')
	raw := []byte(`schema = "https://vimpin.io/schema/v1"

# A comment that should survive.
[[plugin]]
repo = "owner/foo"
commit = "` + hashOld + `"
tag = "v1.0"
`)
	out, missing, err := UpdateCommits(raw, map[string]string{"owner/foo": hashNew})
	if err != nil {
		t.Fatalf("UpdateCommits: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want empty", missing)
	}
	s := string(out)
	if !strings.Contains(s, hashNew) {
		t.Errorf("output missing new hash:\n%s", s)
	}
	if strings.Contains(s, hashOld) {
		t.Errorf("output still contains old hash:\n%s", s)
	}
	if !strings.Contains(s, "# A comment that should survive.") {
		t.Errorf("comment was lost:\n%s", s)
	}
	if !strings.Contains(s, `tag = "v1.0"`) {
		t.Errorf("tag line was lost:\n%s", s)
	}
}

func TestUpdateCommitsInsertsWhenMissing(t *testing.T) {
	hash := makeHash('d')
	raw := []byte(`schema = "https://vimpin.io/schema/v1"

[[plugin]]
repo = "owner/bar"
tag = "v2.0"
`)
	out, _, err := UpdateCommits(raw, map[string]string{"owner/bar": hash})
	if err != nil {
		t.Fatalf("UpdateCommits: %v", err)
	}
	s := string(out)
	repoIdx := strings.Index(s, `repo = "owner/bar"`)
	commitIdx := strings.Index(s, `commit = "`+hash+`"`)
	tagIdx := strings.Index(s, `tag = "v2.0"`)
	if repoIdx < 0 || commitIdx < 0 || tagIdx < 0 {
		t.Fatalf("unexpected output:\n%s", s)
	}
	if !(repoIdx < commitIdx && commitIdx < tagIdx) {
		t.Errorf("insertion order wrong: repo=%d commit=%d tag=%d\n%s", repoIdx, commitIdx, tagIdx, s)
	}
}

func TestUpdateCommitsMultipleBlocks(t *testing.T) {
	hashA := makeHash('a')
	hashB := makeHash('b')
	hashNew := makeHash('e')
	raw := []byte(`schema = "https://vimpin.io/schema/v1"

[[plugin]]
repo = "owner/foo"
commit = "` + hashA + `"

[[plugin]]
repo = "owner/bar"
commit = "` + hashB + `"
`)
	out, _, err := UpdateCommits(raw, map[string]string{"owner/bar": hashNew})
	if err != nil {
		t.Fatalf("UpdateCommits: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, hashA) {
		t.Errorf("owner/foo hash should be preserved:\n%s", s)
	}
	if strings.Contains(s, hashB) {
		t.Errorf("owner/bar old hash should be gone:\n%s", s)
	}
	if !strings.Contains(s, hashNew) {
		t.Errorf("owner/bar new hash should be present:\n%s", s)
	}
}

func TestUpdateCommitsReportsMissing(t *testing.T) {
	raw := []byte(`schema = "https://vimpin.io/schema/v1"

[[plugin]]
repo = "owner/foo"
commit = "` + makeHash('a') + `"
`)
	_, missing, err := UpdateCommits(raw, map[string]string{"owner/notpresent": makeHash('c')})
	if err != nil {
		t.Fatalf("UpdateCommits: %v", err)
	}
	if len(missing) != 1 || missing[0] != "owner/notpresent" {
		t.Errorf("missing = %v, want [owner/notpresent]", missing)
	}
}

func TestUpdateCommitsStopsAtNextSection(t *testing.T) {
	hashA := makeHash('a')
	hashNew := makeHash('e')
	// commit line that lives in a later [settings.something] section must not
	// be confused with the plugin's commit.
	raw := []byte(`schema = "https://vimpin.io/schema/v1"

[[plugin]]
repo = "owner/foo"
tag = "v1.0"

[settings]
default_host = "github.com"
# commit = "should-not-be-touched"

[[plugin]]
repo = "owner/bar"
commit = "` + hashA + `"
`)
	out, _, err := UpdateCommits(raw, map[string]string{"owner/foo": hashNew})
	if err != nil {
		t.Fatalf("UpdateCommits: %v", err)
	}
	s := string(out)
	// foo block should now have commit
	fooBlock := s[strings.Index(s, `repo = "owner/foo"`):strings.Index(s, "[settings]")]
	if !strings.Contains(fooBlock, hashNew) {
		t.Errorf("foo block missing new hash:\n%s", fooBlock)
	}
	// bar's original hash must remain
	if !strings.Contains(s, hashA) {
		t.Errorf("bar's hash should remain:\n%s", s)
	}
}
