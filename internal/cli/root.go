package cli

import (
	"github.com/spf13/cobra"
)

const defaultManifestPath = "vimpin.toml"

func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "vimpin",
		Short: "A universal version pinner for Vim/Neovim plugins",
		Long: `vimpin is a universal version pinner for Vim/Neovim plugins.

It pins plugins to explicit commit hashes via a TOML manifest, integrates
with Renovate for auditable updates, and generates plugin-manager-specific
specs through pluggable adapters.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newPinCmd())
	root.AddCommand(newGenerateCmd())
	root.AddCommand(newVerifyCmd())

	return root
}
