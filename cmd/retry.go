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

var withDeps bool

func init() {
	retryCmd.Flags().BoolVar(&withDeps, "with-deps", false,
		"Also re-run upstream dependency tasks (e.g., git-clone) to refresh workspace data")
	rootCmd.AddCommand(retryCmd)
}

var retryCmd = &cobra.Command{
	Use:   "retry [pipeline-run-name]",
	Short: "Interactively select and retry failed tasks",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRetry,
}

func runRetry(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := tekton.NewClient(namespace)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	pipelineRunName, err := resolvePipelineRun(ctx, client, args)
	if err != nil {
		return err
	}
	if pipelineRunName == "" {
		fmt.Println("Cancelled.")
		return nil
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

	selectorModel := tui.NewTaskSelector(failedTasks, pipelineRunName)
	p := tea.NewProgram(selectorModel, tea.WithAltScreen())
	fm, err := p.Run()
	if err != nil {
		return fmt.Errorf("running task selector: %w", err)
	}
	result := fm.(tui.TaskSelectorModel)
	if !result.Confirmed() {
		fmt.Println("Cancelled.")
		return nil
	}

	selectedTasks := result.SelectedTasks()
	details := []string{fmt.Sprintf("Pipeline Run: %s", pipelineRunName)}
	for _, t := range selectedTasks {
		details = append(details, fmt.Sprintf("Task: %s", t))
	}
	confirmModel := tui.NewConfirm(fmt.Sprintf("Retry %d failed task(s)?", len(selectedTasks)), details)
	p2 := tea.NewProgram(confirmModel, tea.WithAltScreen())
	fm2, err := p2.Run()
	if err != nil {
		return fmt.Errorf("running confirmation: %w", err)
	}
	if !fm2.(tui.ConfirmModel).Confirmed() {
		fmt.Println("Cancelled.")
		return nil
	}

	var results []tekton.RetryResult
	if withDeps {
		fmt.Fprintf(os.Stderr, "Retrying %d task(s) with dependencies...\n", len(selectedTasks))
		results, err = client.RetryWithDeps(ctx, pipelineRunName, selectedTasks)
	} else {
		fmt.Fprintf(os.Stderr, "Retrying %d task(s)...\n", len(selectedTasks))
		results, err = client.RetryFailedTasks(ctx, pipelineRunName, selectedTasks)
	}
	if err != nil {
		return fmt.Errorf("retrying tasks: %w", err)
	}
	for _, r := range results {
		if r.Success {
			fmt.Printf("  OK %s -> %s\n", r.TaskName, r.NewRunName)
		} else {
			fmt.Printf("  FAIL %s: %s\n", r.TaskName, r.Message)
		}
	}
	return nil
}

func resolvePipelineRun(ctx context.Context, client *tekton.Client, args []string) (string, error) {
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
