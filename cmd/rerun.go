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

func init() {
	rootCmd.AddCommand(rerunCmd)
}

var rerunCmd = &cobra.Command{
	Use:   "rerun [pipeline-run-name]",
	Short: "Re-run an entire pipeline run from scratch",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRerun,
}

func runRerun(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := tekton.NewClient(namespace)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	pipelineRunName, err := resolvePipelineRunForRerun(ctx, client, args)
	if err != nil {
		return err
	}
	if pipelineRunName == "" {
		fmt.Println("Cancelled.")
		return nil
	}

	msg := fmt.Sprintf("Re-run entire pipeline %q?", pipelineRunName)
	detail := fmt.Sprintf("Creates a new PipelineRun based on %q", pipelineRunName)
	confirmModel := tui.NewConfirm(msg, []string{detail})
	p := tea.NewProgram(confirmModel, tea.WithAltScreen())
	fm, err := p.Run()
	if err != nil {
		return fmt.Errorf("running confirmation: %w", err)
	}
	if !fm.(tui.ConfirmModel).Confirmed() {
		fmt.Println("Cancelled.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Creating new PipelineRun from %q...\n", pipelineRunName)
	result, err := client.RetryPipelineRun(ctx, pipelineRunName)
	if err != nil {
		return fmt.Errorf("creating retry run: %w", err)
	}
	if result.Success {
		fmt.Printf("OK %s\n", result.Message)
	} else {
		fmt.Printf("FAIL %s\n", result.Message)
	}
	return nil
}

func resolvePipelineRunForRerun(ctx context.Context, client *tekton.Client, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	fmt.Fprintf(os.Stderr, "Fetching pipeline runs from namespace %q...\n", client.Namespace())
	runs, err := client.ListPipelineRuns(ctx, 20)
	if err != nil {
		return "", fmt.Errorf("fetching pipeline runs: %w", err)
	}
	listModel := tui.NewPipelineRunList(runs)
	p := tea.NewProgram(listModel, tea.WithAltScreen())
	fm, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("running TUI: %w", err)
	}
	selected := fm.(tui.PipelineRunListModel).Selected()
	if selected == nil {
		return "", nil
	}
	return selected.Name, nil
}
