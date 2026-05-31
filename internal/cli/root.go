package cli

import (
	"github.com/spf13/cobra"
)

func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "vimpin",
		Short: "Pin Vim/Neovim plugin specs to explicit commit hashes",
		Long: `vimpin pins every plugin in your lazy.nvim spec to an explicit commit hash,
inline. It rewrites your existing Lua specs to the canonical form

  { "owner/repo", commit = "<40-hex>" }, -- tag: <ref>

The commit hash is the source of truth: --verify corrects any drift in the
annotation comment against the SHA, --update bumps to the latest tag. The
annotation comment is treated as a derived artefact, never as a moving
source of intent.

Currently only lazy.nvim specs are supported. Support for additional plugin
managers is planned; see the project README.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newRunCmd())

	return root
}
