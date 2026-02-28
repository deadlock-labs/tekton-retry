package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	namespace string
	rootCmd   = &cobra.Command{
		Use:   "pipeline-retry",
		Short: "Interactive CLI for retrying Tekton pipeline runs",
		Long: `pipeline-retry is an interactive CLI tool that helps you:
  • List recent Tekton PipelineRuns
  • Fuzzy-search and select failed tasks
  • Retry individual tasks or entire pipeline runs

It connects to your current Kubernetes context and namespace.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "",
		"Kubernetes namespace (defaults to current context namespace)")
}

// Execute runs the root command.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}
