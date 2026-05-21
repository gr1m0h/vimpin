package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/gr1m0h/vimpin/internal/adapter"
	_ "github.com/gr1m0h/vimpin/internal/adapter/lazy"
	"github.com/gr1m0h/vimpin/internal/manifest"
	"github.com/spf13/cobra"
)

func newGenerateCmd() *cobra.Command {
	var (
		manifestPath string
		adapterName  string
		groups       []string
		output       string
	)
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate plugin-manager-specific spec from manifest",
		Long: `Generate renders the manifest through a registered adapter and writes the
result to stdout (or --output) so it can be checked into the configuration
directory consumed by the target plugin manager.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(manifestPath, adapterName, groups, output)
		},
	}
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", defaultManifestPath, "Path to manifest file")
	cmd.Flags().StringVarP(&adapterName, "adapter", "a", "lazy", "Adapter to use (one of: "+joinAdapterNames()+")")
	cmd.Flags().StringSliceVarP(&groups, "groups", "g", nil, "Filter by groups (comma-separated). Empty means all groups.")
	cmd.Flags().StringVarP(&output, "output", "o", "-", "Output file (- for stdout)")
	return cmd
}

func runGenerate(path, adapterName string, groups []string, output string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	m, err := manifest.Parse(raw)
	if err != nil {
		return err
	}
	if err := m.Validate(); err != nil {
		return err
	}

	a, err := adapter.Get(adapterName)
	if err != nil {
		return err
	}

	plugins := adapter.FilterByGroups(m.Plugins, groups)

	w, closeFn, err := openOutput(output)
	if err != nil {
		return err
	}
	defer closeFn()

	return a.Generate(m, plugins, w)
}

func openOutput(path string) (io.Writer, func() error, error) {
	if path == "" || path == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create output: %w", err)
	}
	return f, f.Close, nil
}

func joinAdapterNames() string {
	names := adapter.Names()
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
