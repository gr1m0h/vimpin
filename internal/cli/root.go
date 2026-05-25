package cli

import (
	"github.com/spf13/cobra"
)

func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "vimpin",
		Short: "A pinact-style commit pinner for Vim/Neovim plugin managers",
		Long: `vimpin pins every plugin in your lazy.nvim spec to an explicit commit hash,
inline. It rewrites your existing Lua specs to the canonical form

  { "owner/repo", commit = "<40-hex>" }, -- tag: <ref>

and ships a Renovate preset so the commit is bumped via reviewable pull
requests when the tracked tag or branch moves.

Currently only lazy.nvim specs are supported. Support for additional plugin
managers is planned; see the project README.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newRunCmd())
	root.AddCommand(newVerifyCmd())

	return root
}
