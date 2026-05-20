package manifest

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

var (
	pluginHeaderRe = regexp.MustCompile(`^\s*\[\[plugin\]\]\s*$`)
	tableHeaderRe  = regexp.MustCompile(`^\s*\[[^\[]`)
	repoLineRe     = regexp.MustCompile(`^\s*repo\s*=\s*"([^"]+)"\s*$`)
	commitLineRe   = regexp.MustCompile(`^(\s*commit\s*=\s*")[^"]*("\s*)$`)
)

// UpdateCommits rewrites the raw manifest, replacing the commit field for each
// plugin whose repo is a key in updates. If a plugin has no commit line, one
// is inserted immediately after its repo line. The rest of the file
// (comments, spacing, field order) is preserved verbatim.
//
// Repos that appear in updates but not in the manifest are returned in the
// missing slice; no error is raised so callers can decide how to handle them.
func UpdateCommits(raw []byte, updates map[string]string) (result []byte, missing []string, err error) {
	if len(updates) == 0 {
		return append([]byte(nil), raw...), nil, nil
	}

	lines := splitLines(raw)
	out := make([]string, 0, len(lines))

	applied := make(map[string]bool, len(updates))

	// Scan plugin blocks. A block starts at a [[plugin]] line and ends at the
	// next [[plugin]] line or any other [section] header, or EOF.
	i := 0
	for i < len(lines) {
		line := lines[i]
		if !pluginHeaderRe.MatchString(line) {
			out = append(out, line)
			i++
			continue
		}

		// Collect the block.
		blockStart := i
		blockEnd := i + 1
		for blockEnd < len(lines) {
			ln := lines[blockEnd]
			if pluginHeaderRe.MatchString(ln) || tableHeaderRe.MatchString(ln) {
				break
			}
			blockEnd++
		}
		block := lines[blockStart:blockEnd]

		updated, repo, ok := updateBlock(block, updates)
		if ok {
			applied[repo] = true
		}
		out = append(out, updated...)
		i = blockEnd
	}

	for repo := range updates {
		if !applied[repo] {
			missing = append(missing, repo)
		}
	}

	var buf bytes.Buffer
	for i, ln := range out {
		buf.WriteString(ln)
		if i < len(out)-1 || endsWithNewline(raw) {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes(), missing, nil
}

// updateBlock processes a single [[plugin]] block. The first line is the
// header. Returns the rewritten block, the repo that was matched (if any),
// and whether an update was applied.
func updateBlock(block []string, updates map[string]string) ([]string, string, bool) {
	var repo string
	repoIdx := -1
	commitIdx := -1
	for i, ln := range block {
		if m := repoLineRe.FindStringSubmatch(ln); m != nil {
			repo = m[1]
			repoIdx = i
		}
		if commitLineRe.MatchString(ln) {
			commitIdx = i
		}
	}
	if repo == "" {
		return block, "", false
	}
	newCommit, ok := updates[repo]
	if !ok {
		return block, "", false
	}

	if commitIdx >= 0 {
		block[commitIdx] = commitLineRe.ReplaceAllString(block[commitIdx], "${1}"+newCommit+"${2}")
		return block, repo, true
	}
	// Insert commit line immediately after the repo line.
	insertAt := repoIdx + 1
	indent := leadingIndent(block[repoIdx])
	newLine := fmt.Sprintf("%scommit = %q", indent, newCommit)
	out := make([]string, 0, len(block)+1)
	out = append(out, block[:insertAt]...)
	out = append(out, newLine)
	out = append(out, block[insertAt:]...)
	return out, repo, true
}

func splitLines(raw []byte) []string {
	s := string(raw)
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func endsWithNewline(raw []byte) bool {
	return len(raw) > 0 && raw[len(raw)-1] == '\n'
}

func leadingIndent(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s
}
