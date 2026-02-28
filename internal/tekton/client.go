package tekton

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	pipelineRunGVR = schema.GroupVersionResource{
		Group:    "tekton.dev",
		Version:  "v1",
		Resource: "pipelineruns",
	}
	taskRunGVR = schema.GroupVersionResource{
		Group:    "tekton.dev",
		Version:  "v1",
		Resource: "taskruns",
	}
	pipelineGVR = schema.GroupVersionResource{
		Group:    "tekton.dev",
		Version:  "v1",
		Resource: "pipelines",
	}
)

// Client wraps dynamic Kubernetes access for Tekton resources.
type Client struct {
	dyn       dynamic.Interface
	namespace string
}

// NewClient creates a Tekton client using the current kubeconfig context.
func NewClient(namespace string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})

	restCfg, err := config.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	if namespace == "" {
		namespace, _, err = config.Namespace()
		if err != nil {
			return nil, fmt.Errorf("resolving namespace: %w", err)
		}
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	return &Client{dyn: dynClient, namespace: namespace}, nil
}

// Namespace returns the resolved namespace.
func (c *Client) Namespace() string {
	return c.namespace
}

// ListPipelineRuns returns recent pipeline runs sorted newest-first.
func (c *Client) ListPipelineRuns(ctx context.Context, limit int) ([]PipelineRunInfo, error) {
	list, err := c.dyn.Resource(pipelineRunGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pipelineruns: %w", err)
	}

	var runs []PipelineRunInfo
	for _, item := range list.Items {
		runs = append(runs, parsePipelineRun(item))
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartTime.After(runs[j].StartTime)
	})

	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

// GetPipelineRun returns a single pipeline run by name.
func (c *Client) GetPipelineRun(ctx context.Context, name string) (*PipelineRunInfo, error) {
	obj, err := c.dyn.Resource(pipelineRunGVR).Namespace(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pipelinerun %q: %w", name, err)
	}
	info := parsePipelineRun(*obj)
	return &info, nil
}

// ListTaskRunsForPipeline returns task runs that belong to a specific pipeline run.
func (c *Client) ListTaskRunsForPipeline(ctx context.Context, pipelineRunName string) ([]TaskRunInfo, error) {
	labelSelector := fmt.Sprintf("tekton.dev/pipelineRun=%s", pipelineRunName)
	list, err := c.dyn.Resource(taskRunGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("listing taskruns for %q: %w", pipelineRunName, err)
	}

	var tasks []TaskRunInfo
	for _, item := range list.Items {
		tasks = append(tasks, parseTaskRun(item, pipelineRunName))
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].StartTime.Before(tasks[j].StartTime)
	})
	return tasks, nil
}

// GetFailedTaskRuns filters task runs to only failed ones.
func (c *Client) GetFailedTaskRuns(ctx context.Context, pipelineRunName string) ([]TaskRunInfo, error) {
	tasks, err := c.ListTaskRunsForPipeline(ctx, pipelineRunName)
	if err != nil {
		return nil, err
	}

	var failed []TaskRunInfo
	for _, t := range tasks {
		if t.Status == "Failed" {
			failed = append(failed, t)
		}
	}
	return failed, nil
}

// RetryPipelineRun creates a new PipelineRun based on an existing (failed) one.
func (c *Client) RetryPipelineRun(ctx context.Context, originalRunName string) (*RetryResult, error) {
	original, err := c.dyn.Resource(pipelineRunGVR).Namespace(c.namespace).Get(ctx, originalRunName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting original pipelinerun %q: %w", originalRunName, err)
	}

	newRun := buildRetryPipelineRun(original)
	created, err := c.dyn.Resource(pipelineRunGVR).Namespace(c.namespace).Create(ctx, newRun, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating retry pipelinerun: %w", err)
	}

	return &RetryResult{
		OriginalRun: originalRunName,
		NewRunName:  created.GetName(),
		Success:     true,
		Message:     fmt.Sprintf("Created retry PipelineRun %q from %q", created.GetName(), originalRunName),
	}, nil
}

// RetryFailedTasks creates individual TaskRuns for each failed task in a pipeline run.
func (c *Client) RetryFailedTasks(ctx context.Context, pipelineRunName string, taskNames []string) ([]RetryResult, error) {
	failedTasks, err := c.GetFailedTaskRuns(ctx, pipelineRunName)
	if err != nil {
		return nil, err
	}

	selected := make(map[string]bool)
	for _, name := range taskNames {
		selected[name] = true
	}

	var results []RetryResult
	for _, task := range failedTasks {
		if len(taskNames) > 0 && !selected[task.TaskName] {
			continue
		}

		result := RetryResult{
			OriginalRun: pipelineRunName,
			TaskName:    task.TaskName,
		}

		// Get the original TaskRun to use as a template
		original, err := c.dyn.Resource(taskRunGVR).Namespace(c.namespace).Get(ctx, task.Name, metav1.GetOptions{})
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to get original taskrun: %v", err)
			results = append(results, result)
			continue
		}

		newTaskRun := buildRetryTaskRun(original)
		created, err := c.dyn.Resource(taskRunGVR).Namespace(c.namespace).Create(ctx, newTaskRun, metav1.CreateOptions{})
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to create retry taskrun: %v", err)
			results = append(results, result)
			continue
		}

		result.NewRunName = created.GetName()
		result.Success = true
		result.Message = fmt.Sprintf("Created retry TaskRun %q for task %q", created.GetName(), task.TaskName)
		results = append(results, result)
	}
	return results, nil
}

// RetryWithDeps retries the selected tasks AND their upstream workspace-producing dependencies.
// It reads the Pipeline spec to discover runAfter/workspace relationships, finds ancestor tasks
// that produce workspace data needed by the selected tasks, and re-runs those first.
func (c *Client) RetryWithDeps(ctx context.Context, pipelineRunName string, taskNames []string) ([]RetryResult, error) {
	// 1. Get the PipelineRun to find which Pipeline it references
	prObj, err := c.dyn.Resource(pipelineRunGVR).Namespace(c.namespace).Get(ctx, pipelineRunName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pipelinerun %q: %w", pipelineRunName, err)
	}

	pipelineName := extractPipelineName(prObj)
	if pipelineName == "" {
		// Can't resolve deps without Pipeline spec, fall back to plain retry
		return c.RetryFailedTasks(ctx, pipelineRunName, taskNames)
	}

	// 2. Get the Pipeline to read the task DAG
	pipelineObj, err := c.dyn.Resource(pipelineGVR).Namespace(c.namespace).Get(ctx, pipelineName, metav1.GetOptions{})
	if err != nil {
		// Pipeline might be cluster-scoped or missing — fall back
		return c.RetryFailedTasks(ctx, pipelineRunName, taskNames)
	}

	// 3. Build dependency graph from the Pipeline spec
	depGraph := buildDependencyGraph(pipelineObj)

	// 4. For each selected task, find all upstream deps (transitive)
	allTasks := make(map[string]bool)
	for _, name := range taskNames {
		allTasks[name] = true
		for _, dep := range getTransitiveDeps(depGraph, name) {
			allTasks[dep] = true
		}
	}

	// 5. Get all TaskRuns for this PipelineRun
	allTaskRuns, err := c.ListTaskRunsForPipeline(ctx, pipelineRunName)
	if err != nil {
		return nil, err
	}

	// 6. Build a name→TaskRunInfo map
	trMap := make(map[string]TaskRunInfo)
	for _, tr := range allTaskRuns {
		trMap[tr.TaskName] = tr
	}

	// 7. Topologically sort the tasks to retry and execute in order
	ordered := topoSort(depGraph, allTasks)

	var results []RetryResult
	for _, taskName := range ordered {
		tr, exists := trMap[taskName]
		if !exists {
			results = append(results, RetryResult{
				OriginalRun: pipelineRunName,
				TaskName:    taskName,
				Success:     false,
				Message:     "No original TaskRun found for this task",
			})
			continue
		}

		result := RetryResult{OriginalRun: pipelineRunName, TaskName: taskName}

		original, err := c.dyn.Resource(taskRunGVR).Namespace(c.namespace).Get(ctx, tr.Name, metav1.GetOptions{})
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to get original taskrun: %v", err)
			results = append(results, result)
			continue
		}

		newTaskRun := buildRetryTaskRun(original)
		created, err := c.dyn.Resource(taskRunGVR).Namespace(c.namespace).Create(ctx, newTaskRun, metav1.CreateOptions{})
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to create retry taskrun: %v", err)
			results = append(results, result)
			continue
		}

		isDirectTarget := false
		for _, name := range taskNames {
			if name == taskName {
				isDirectTarget = true
				break
			}
		}
		prefix := "(dep)"
		if isDirectTarget {
			prefix = "(target)"
		}

		result.NewRunName = created.GetName()
		result.Success = true
		result.Message = fmt.Sprintf("%s Created retry TaskRun %q for task %q", prefix, created.GetName(), taskName)
		results = append(results, result)
	}
	return results, nil
}

// --- dependency graph helpers ---

// extractPipelineName gets the pipeline name from a PipelineRun's spec.pipelineRef.name
func extractPipelineName(prObj *unstructured.Unstructured) string {
	spec, ok := prObj.Object["spec"].(map[string]interface{})
	if !ok {
		return ""
	}
	pRef, ok := spec["pipelineRef"].(map[string]interface{})
	if !ok {
		return ""
	}
	name, _ := pRef["name"].(string)
	return name
}

// depNode represents a task in the Pipeline DAG.
type depNode struct {
	Name     string
	RunAfter []string // explicit runAfter dependencies
}

// buildDependencyGraph reads the Pipeline spec.tasks[] and extracts runAfter relationships.
// Returns map[taskName] -> []upstreamTaskNames
func buildDependencyGraph(pipelineObj *unstructured.Unstructured) map[string][]string {
	graph := make(map[string][]string)

	spec, ok := pipelineObj.Object["spec"].(map[string]interface{})
	if !ok {
		return graph
	}

	tasks, ok := spec["tasks"].([]interface{})
	if !ok {
		return graph
	}

	for _, t := range tasks {
		taskMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := taskMap["name"].(string)
		if name == "" {
			continue
		}

		var deps []string

		// Explicit runAfter
		if runAfter, ok := taskMap["runAfter"].([]interface{}); ok {
			for _, dep := range runAfter {
				if depName, ok := dep.(string); ok {
					deps = append(deps, depName)
				}
			}
		}

		// Implicit deps from workspace "from" — tasks that produce results consumed here
		// Tekton also creates implicit ordering via "$(tasks.X.results.Y)" params
		if params, ok := taskMap["params"].([]interface{}); ok {
			for _, p := range params {
				pMap, ok := p.(map[string]interface{})
				if !ok {
					continue
				}
				val, _ := pMap["value"].(string)
				// Parse $(tasks.<taskname>.results.<resultname>)
				refs := extractTaskRefs(val)
				deps = append(deps, refs...)
			}
		}

		graph[name] = deps
	}

	return graph
}

// extractTaskRefs parses Tekton variable references like $(tasks.git-clone.results.commit)
// and returns the task names referenced.
func extractTaskRefs(value string) []string {
	var refs []string
	remaining := value
	for {
		idx := strings.Index(remaining, "$(tasks.")
		if idx == -1 {
			break
		}
		remaining = remaining[idx+len("$(tasks."):]
		dotIdx := strings.Index(remaining, ".")
		if dotIdx == -1 {
			break
		}
		taskName := remaining[:dotIdx]
		if taskName != "" {
			refs = append(refs, taskName)
		}
		remaining = remaining[dotIdx:]
	}
	return refs
}

// getTransitiveDeps returns all transitive upstream dependencies for a given task.
func getTransitiveDeps(graph map[string][]string, taskName string) []string {
	visited := make(map[string]bool)
	var result []string
	var walk func(name string)
	walk = func(name string) {
		deps, ok := graph[name]
		if !ok {
			return
		}
		for _, dep := range deps {
			if !visited[dep] {
				visited[dep] = true
				result = append(result, dep)
				walk(dep)
			}
		}
	}
	walk(taskName)
	return result
}

// topoSort returns the tasks in topological order (dependencies first).
func topoSort(graph map[string][]string, tasks map[string]bool) []string {
	visited := make(map[string]bool)
	var order []string
	var visit func(name string)
	visit = func(name string) {
		if visited[name] || !tasks[name] {
			return
		}
		visited[name] = true
		// Visit dependencies first
		for _, dep := range graph[name] {
			if tasks[dep] {
				visit(dep)
			}
		}
		order = append(order, name)
	}
	for name := range tasks {
		visit(name)
	}
	return order
}

// --- parsers ---

func parsePipelineRun(obj unstructured.Unstructured) PipelineRunInfo {
	info := PipelineRunInfo{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	// Pipeline reference
	if spec, ok := obj.Object["spec"].(map[string]interface{}); ok {
		if pRef, ok := spec["pipelineRef"].(map[string]interface{}); ok {
			if name, ok := pRef["name"].(string); ok {
				info.Pipeline = name
			}
		}
	}

	// Status & times
	if status, ok := obj.Object["status"].(map[string]interface{}); ok {
		info.Status = extractConditionStatus(status)
		if st, ok := status["startTime"].(string); ok {
			if t, err := time.Parse(time.RFC3339, st); err == nil {
				info.StartTime = t
			}
		}
		if ct, ok := status["completionTime"].(string); ok {
			if t, err := time.Parse(time.RFC3339, ct); err == nil {
				info.Duration = t.Sub(info.StartTime)
			}
		}
	}

	if info.Status == "" {
		info.Status = "Unknown"
	}
	return info
}

func parseTaskRun(obj unstructured.Unstructured, pipelineRunName string) TaskRunInfo {
	info := TaskRunInfo{
		Name:        obj.GetName(),
		PipelineRun: pipelineRunName,
	}

	// Task name from labels
	labels := obj.GetLabels()
	if tn, ok := labels["tekton.dev/pipelineTask"]; ok {
		info.TaskName = tn
	}

	// Status & times
	if status, ok := obj.Object["status"].(map[string]interface{}); ok {
		info.Status = extractConditionStatus(status)
		if st, ok := status["startTime"].(string); ok {
			if t, err := time.Parse(time.RFC3339, st); err == nil {
				info.StartTime = t
			}
		}
		if ct, ok := status["completionTime"].(string); ok {
			if t, err := time.Parse(time.RFC3339, ct); err == nil {
				info.Duration = t.Sub(info.StartTime)
			}
		}

		// Failure reason from steps
		if steps, ok := status["steps"].([]interface{}); ok {
			for _, s := range steps {
				if step, ok := s.(map[string]interface{}); ok {
					if term, ok := step["terminated"].(map[string]interface{}); ok {
						if reason, ok := term["reason"].(string); ok && reason == "Error" {
							if msg, ok := term["message"].(string); ok {
								info.FailureReason = msg
							}
						}
					}
				}
			}
		}
	}

	if info.Status == "" {
		info.Status = "Unknown"
	}
	return info
}

func extractConditionStatus(status map[string]interface{}) string {
	conditions, ok := status["conditions"]
	if !ok {
		return "Unknown"
	}
	condList, ok := conditions.([]interface{})
	if !ok || len(condList) == 0 {
		return "Unknown"
	}
	cond, ok := condList[len(condList)-1].(map[string]interface{})
	if !ok {
		return "Unknown"
	}

	condStatus, _ := cond["status"].(string)
	reason, _ := cond["reason"].(string)

	switch condStatus {
	case "True":
		return "Succeeded"
	case "False":
		if reason != "" {
			return "Failed"
		}
		return "Failed"
	default:
		if reason == "Running" {
			return "Running"
		}
		return "Unknown"
	}
}

func buildRetryPipelineRun(original *unstructured.Unstructured) *unstructured.Unstructured {
	spec, _ := original.Object["spec"].(map[string]interface{})

	newName := safeRetryName(original.GetName())
	labels := original.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["pipeline-retry/original-run"] = original.GetName()
	labels["pipeline-retry/retried-by"] = "pipeline-retry-cli"

	annotations := original.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["pipeline-retry/original-run"] = original.GetName()
	annotations["pipeline-retry/retry-time"] = time.Now().Format(time.RFC3339)

	newRun := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "tekton.dev/v1",
			"kind":       "PipelineRun",
			"metadata": map[string]interface{}{
				"name":        newName,
				"namespace":   original.GetNamespace(),
				"labels":      toMapInterface(labels),
				"annotations": toMapInterface(annotations),
			},
			"spec": deepCopyJSON(spec),
		},
	}
	return newRun
}

func buildRetryTaskRun(original *unstructured.Unstructured) *unstructured.Unstructured {
	spec, _ := original.Object["spec"].(map[string]interface{})

	newName := safeRetryName(original.GetName())
	labels := original.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["pipeline-retry/original-taskrun"] = original.GetName()
	labels["pipeline-retry/retried-by"] = "pipeline-retry-cli"

	newTaskRun := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "tekton.dev/v1",
			"kind":       "TaskRun",
			"metadata": map[string]interface{}{
				"name":      newName,
				"namespace": original.GetNamespace(),
				"labels":    toMapInterface(labels),
			},
			"spec": deepCopyJSON(spec),
		},
	}
	return newTaskRun
}

// safeRetryName generates a retry name that fits within the 63-char K8s limit.
// Format: {truncated-base}-r-{short-timestamp}  (suffix is 12 chars max)
func safeRetryName(baseName string) string {
	suffix := fmt.Sprintf("-r-%d", time.Now().Unix()%1000000) // 6-digit rolling timestamp
	maxBase := 63 - len(suffix)
	if len(baseName) > maxBase {
		baseName = baseName[:maxBase]
	}
	// Trim trailing hyphens from truncation
	baseName = strings.TrimRight(baseName, "-")
	return baseName + suffix
}

func toMapInterface(m map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func deepCopyJSON(obj interface{}) interface{} {
	data, err := json.Marshal(obj)
	if err != nil {
		return obj
	}
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return obj
	}
	return result
}
