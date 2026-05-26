package cli

import "github.com/gr1m0h/vimpin/internal/resolver"

// newResolver returns the resolver used by the run and verify commands.
// It is a package-level variable so tests can swap in a fake resolver
// without touching the network or requiring a configured git binary.
var newResolver = func() resolver.Resolver {
	return resolver.NewGitResolver()
}
