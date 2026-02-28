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
	rootCmd.AddCommand(retryAllCmd)
}

var retryAllCmd = &cobra.Command{
	Use:   "retry-all [pipeline-run-name]",
	Short: "Retry all failed tasks in a pipeline run",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRetryAll,
}

func runRetryAll(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := tekton.NewClient(namespace)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	var pipelineRunName string
	if len(args) > 0 {
		pipelineRunName = args[0]
	} else {
		fmt.Fprintf(os.Stderr, "Fetching failed pipeline runs from namespace %q...\n", client.Namespace())
		runs, err := client.ListPipelineRuns(ctx, 50)
		if err != nil {
			return fmt.Errorf("fetching pipeline runs: %w", err)
		}
		var failedRuns []tekton.PipelineRunInfo
		for _, r := range runs {
			if r.Status == "Failed" {
				failedRuns = append(failedRuns, r)
			}
		}
		if len(failedRuns) == 0 {
			fmt.Println("No failed pipeline runs found.")
			return nil
		}
		listModel := tui.NewPipelineRunList(failedRuns)
		p := tea.NewProgram(listModel, tea.WithAltScreen())
		fm, err := p.Run()
		if err != nil {
			return fmt.Errorf("running TUI: %w", err)
		}
		selected := fm.(tui.PipelineRunListModel).Selected()
		if selected == nil {
			fmt.Println("Cancelled.")
			return nil
		}
		pipelineRunName = selected.Name
	}

	fmt.Fprintf(os.Stderr, "Fetching failed tasks for %q...\n", pipelineRunName)
	failedTasks, err := client.GetFailedTaskRuns(ctx, pipelineRunName)
	if err != nil {
		return fmt.Errorf("fetching failed tasks: %w", err)
	}
	if len(failedTasks) == 0 {
		fmt.Println("No failed tasks found. Nothing to retry.")
		return nil
	}

	fmt.Printf("\nPipeline Run: %s\n", pipelineRunName)
	fmt.Printf("Failed tasks (%d):\n", len(failedTasks))
	details := []string{fmt.Sprintf("Pipeline Run: %s", pipelineRunName)}
	for _, t := range failedTasks {
		fmt.Printf("  X %s\n", t.TaskName)
		details = append(details, fmt.Sprintf("Task: %s", t.TaskName))
	}

	msg := fmt.Sprintf("Retry ALL %d failed task(s)?", len(failedTasks))
	confirmModel := tui.NewConfirm(msg, details)
	p := tea.NewProgram(confirmModel, tea.WithAltScreen())
	fm, err := p.Run()
	if err != nil {
		return fmt.Errorf("running confirmation: %w", err)
	}
	if !fm.(tui.ConfirmModel).Confirmed() {
		fmt.Println("Cancelled.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "\nRetrying %d task(s)...\n", len(failedTasks))
	results, err := client.RetryFailedTasks(ctx, pipelineRunName, nil)
	if err != nil {
		return fmt.Errorf("retrying tasks: %w", err)
	}
	successCount := 0
	for _, r := range results {
		if r.Success {
			fmt.Printf("  OK %s -> %s\n", r.TaskName, r.NewRunName)
			successCount++
		} else {
			fmt.Printf("  FAIL %s: %s\n", r.TaskName, r.Message)
		}
	}
	fmt.Printf("\nDone: %d/%d tasks retried successfully.\n", successCount, len(results))
	return nil
}
