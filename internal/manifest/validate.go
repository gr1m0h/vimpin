package manifest

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var commitHashPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ValidateError aggregates per-plugin validation errors.
type ValidateError struct {
	Errors []string
}

func (e *ValidateError) Error() string {
	return strings.Join(e.Errors, "\n")
}

// Validate performs structural checks against the manifest. It does not
// reach out to any network; use the resolver package for that.
func (m *Manifest) Validate() error {
	var errs []string

	if m.Schema == "" {
		errs = append(errs, "schema: missing top-level schema field")
	} else if m.Schema != SchemaV1 {
		errs = append(errs, fmt.Sprintf("schema: unsupported schema %q (expected %q)", m.Schema, SchemaV1))
	}

	if len(m.Plugins) == 0 {
		errs = append(errs, "plugin: manifest has no [[plugin]] entries")
	}

	allowedHosts := make(map[string]bool, len(m.Settings.AllowHosts))
	for _, h := range m.Settings.AllowHosts {
		allowedHosts[h] = true
	}
	// If allow_hosts is unset, fall back to allowing github.com.
	if len(allowedHosts) == 0 {
		allowedHosts["github.com"] = true
	}

	seen := make(map[string]bool, len(m.Plugins))
	for i, p := range m.Plugins {
		prefix := fmt.Sprintf("plugin[%d] (%s)", i, p.Repo)

		if p.Repo == "" {
			errs = append(errs, fmt.Sprintf("plugin[%d]: missing repo field", i))
			continue
		}
		if !strings.Contains(p.Repo, "/") {
			errs = append(errs, fmt.Sprintf("%s: repo must be in owner/name form", prefix))
		}
		if seen[p.Repo] {
			errs = append(errs, fmt.Sprintf("%s: duplicate repo entry", prefix))
		}
		seen[p.Repo] = true

		host := p.EffectiveHost(m.Settings)
		if !allowedHosts[host] {
			errs = append(errs, fmt.Sprintf("%s: host %q not in settings.allow_hosts", prefix, host))
		}

		if p.Layer != "" && p.Layer != "user" && p.Layer != "override" {
			errs = append(errs, fmt.Sprintf("%s: layer must be \"user\" or \"override\", got %q", prefix, p.Layer))
		}

		if p.Commit == "" && p.Tag == "" && p.Branch == "" {
			errs = append(errs, fmt.Sprintf("%s: must have at least one of commit, tag, branch", prefix))
		}
		if p.Commit != "" && !commitHashPattern.MatchString(p.Commit) {
			errs = append(errs, fmt.Sprintf("%s: commit %q is not a 40-character lowercase hex hash", prefix, p.Commit))
		}
	}

	if len(errs) > 0 {
		return &ValidateError{Errors: errs}
	}
	return nil
}

// IsValidateError reports whether err is a *ValidateError.
func IsValidateError(err error) bool {
	var v *ValidateError
	return errors.As(err, &v)
}
