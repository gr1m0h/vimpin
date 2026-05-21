package adapter

import (
	"reflect"
	"sort"
	"testing"

	"github.com/gr1m0h/vimpin/internal/manifest"
)

func TestFilterByGroups(t *testing.T) {
	plugins := []manifest.Plugin{
		{Repo: "a/1", Group: "core"},
		{Repo: "a/2", Group: "work"},
		{Repo: "a/3"}, // implicit "default"
	}
	cases := map[string][]string{
		"": {"a/1", "a/2", "a/3"},
		// passing no groups is represented by nil; test by calling directly.
	}
	for groupFilter, want := range cases {
		var keep []string
		if groupFilter != "" {
			keep = []string{groupFilter}
		}
		got := nameSlice(FilterByGroups(plugins, keep))
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("FilterByGroups(%q) = %v, want %v", groupFilter, got, want)
		}
	}

	got := nameSlice(FilterByGroups(plugins, []string{"core"}))
	if !reflect.DeepEqual(got, []string{"a/1"}) {
		t.Errorf("core filter = %v, want [a/1]", got)
	}

	got = nameSlice(FilterByGroups(plugins, []string{"default"}))
	if !reflect.DeepEqual(got, []string{"a/3"}) {
		t.Errorf("default filter = %v, want [a/3]", got)
	}

	got = nameSlice(FilterByGroups(plugins, []string{"core", "work"}))
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"a/1", "a/2"}) {
		t.Errorf("core+work filter = %v, want [a/1 a/2]", got)
	}
}

func nameSlice(ps []manifest.Plugin) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Repo
	}
	return out
}
