package tekton

import "time"

// PipelineRunInfo holds summarized pipeline run data.
type PipelineRunInfo struct {
	Name      string
	Namespace string
	Pipeline  string
	Status    string // Succeeded, Failed, Running, Unknown
	StartTime time.Time
	Duration  time.Duration
	Tasks     []TaskRunInfo
}

// TaskRunInfo holds summarized task run data.
type TaskRunInfo struct {
	Name          string
	TaskName      string
	PipelineRun   string
	Status        string // Succeeded, Failed, Running, Unknown
	StartTime     time.Time
	Duration      time.Duration
	FailureReason string
}

// RetryResult captures the outcome of a retry operation.
type RetryResult struct {
	OriginalRun string
	NewRunName  string
	TaskName    string
	Success     bool
	Message     string
}
