package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVerifyPassesOnPinnedManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vimpin.toml")
	content := fmt.Sprintf(`schema = "https://vimpin.io/schema/v1"

[settings]
default_host = "github.com"
allow_hosts = ["github.com"]

[[plugin]]
repo = "owner/foo"
commit = "%s"
tag = "v1.0"
`, strings.Repeat("a", 40))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := runVerify(context.Background(), path, false); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestRunVerifyFailsOnMissingCommit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vimpin.toml")
	content := `schema = "https://vimpin.io/schema/v1"

[settings]
allow_hosts = ["github.com"]

[[plugin]]
repo = "owner/foo"
tag = "v1.0"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runVerify(context.Background(), path, false)
	if err == nil {
		t.Fatal("expected verify failure")
	}
	if !strings.Contains(err.Error(), "commit field is required") {
		t.Errorf("error %q should mention missing commit", err.Error())
	}
}

func TestRunVerifyStrictDetectsDrift(t *testing.T) {
	writeFakeGitForCLI(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "vimpin.toml")
	stale := strings.Repeat("c", 40)
	content := fmt.Sprintf(`schema = "https://vimpin.io/schema/v1"

[settings]
allow_hosts = ["github.com"]

[[plugin]]
repo = "owner/foo"
commit = "%s"
tag = "v1.0"
`, stale)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runVerify(context.Background(), path, true)
	if err == nil {
		t.Fatal("expected verify --strict failure due to tag drift")
	}
	if !strings.Contains(err.Error(), "no longer matches") {
		t.Errorf("error %q should mention drift", err.Error())
	}
}

func TestRunVerifyStrictPassesWhenAligned(t *testing.T) {
	writeFakeGitForCLI(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "vimpin.toml")
	content := fmt.Sprintf(`schema = "https://vimpin.io/schema/v1"

[settings]
allow_hosts = ["github.com"]

[[plugin]]
repo = "owner/foo"
commit = "%s"
tag = "v1.0"
`, strings.Repeat("a", 40))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := runVerify(context.Background(), path, true); err != nil {
		t.Fatalf("verify --strict: %v", err)
	}
}
