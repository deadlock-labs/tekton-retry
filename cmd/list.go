package cmd

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ford-mstech/pipeline-retry/internal/tekton"
	"github.com/ford-mstech/pipeline-retry/internal/tui"
	"github.com/spf13/cobra"
)

var listLimit int

func init() {
	listCmd.Flags().IntVarP(&listLimit, "limit", "l", 20, "Max pipeline runs to show")
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent pipeline runs interactively",
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := tekton.NewClient(namespace)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Fetching pipeline runs from namespace %q...\n", client.Namespace())
	runs, err := client.ListPipelineRuns(ctx, listLimit)
	if err != nil {
		return fmt.Errorf("fetching pipeline runs: %w", err)
	}
	listModel := tui.NewPipelineRunList(runs)
	p := tea.NewProgram(listModel, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}
	result := finalModel.(tui.PipelineRunListModel)
	selected := result.Selected()
	if selected == nil {
		fmt.Println("No pipeline run selected.")
		return nil
	}
	fmt.Fprintf(os.Stderr, "\nFetching tasks for %q...\n", selected.Name)
	tasks, err := client.ListTaskRunsForPipeline(ctx, selected.Name)
	if err != nil {
		return fmt.Errorf("fetching tasks: %w", err)
	}
	fmt.Printf("\nPipeline Run: %s\n", selected.Name)
	fmt.Printf("Pipeline:     %s\n", selected.Pipeline)
	fmt.Printf("Status:       %s\n", selected.Status)
	fmt.Printf("Tasks:        %d\n\n", len(tasks))
	for _, t := range tasks {
		icon := tui.StatusIcon(t.Status)
		fmt.Printf("  %s %-30s %s", icon, t.TaskName, t.Status)
		if t.FailureReason != "" {
			fmt.Printf("  (%s)", t.FailureReason)
		}
		fmt.Println()
	}
	return nil
}
