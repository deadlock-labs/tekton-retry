# pipeline-retry — Technical Deep Dive

This document explains **how the tool works at the code level**: how it connects to
Kubernetes, discovers Tekton resources, renders the interactive TUI, and creates
new Runs to trigger retries.

---

## 1. Architecture Overview

```
 ┌──────────────┐      ┌──────────────────┐      ┌─────────────────────┐
 │  CLI Layer   │─────▶│   TUI Layer      │      │  Tekton Client      │
 │  (cmd/*.go)  │      │  (tui/*.go)      │      │  (tekton/client.go) │
 │  cobra cmds  │      │  bubbletea models│      │  dynamic K8s client │
 └──────┬───────┘      └──────────────────┘      └──────────┬──────────┘
        │                                                    │
        │         ┌─────────────────────────┐               │
        └────────▶│  Kubernetes API Server  │◀──────────────┘
                  │  (Tekton CRDs)          │
                  └─────────────────────────┘
```

**Flow:** CLI command → fetch data via Tekton client → present in TUI → user selects → create new K8s resource → Tekton controller picks it up and runs it.

---

## 2. Connecting to the Cluster

**File:** `internal/tekton/client.go` → `NewClient()`

```go
func NewClient(namespace string) (*Client, error) {
    rules := clientcmd.NewDefaultClientConfigLoadingRules()
    config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
    restCfg, err := config.ClientConfig()
    // ...
    dynClient, err := dynamic.NewForConfig(restCfg)
    // ...
}
```

### What happens:
1. **`clientcmd.NewDefaultClientConfigLoadingRules()`** — Loads your kubeconfig from the
   standard locations (`$KUBECONFIG`, `~/.kube/config`). Same file `kubectl` uses.

2. **`config.ClientConfig()`** — Resolves the **current context** (cluster + auth) into a
   REST config (server URL, TLS certs, bearer token, etc.).

3. **`config.Namespace()`** — If `-n` flag is not provided, reads the default namespace
   from your kubeconfig context (the one `kubectl config view --minify` shows).

4. **`dynamic.NewForConfig(restCfg)`** — Creates a **dynamic client** instead of a typed client.
   This is key: we don't need generated Tekton Go types. The dynamic client works with
   `unstructured.Unstructured` (basically `map[string]interface{}`), which lets us read/write
   any CRD using just its Group-Version-Resource (GVR).

### Why dynamic client?
- No dependency on Tekton's Go SDK (which has heavy transitive deps)
- Works with any Tekton API version
- The tool only needs a few fields, not the full typed struct

---

## 3. Querying Tekton CRDs

Tekton installs Custom Resource Definitions in your cluster. The two we care about:

| CRD | GVR | What it is |
|-----|-----|------------|
| `PipelineRun` | `tekton.dev/v1/pipelineruns` | A single execution of a Pipeline |
| `TaskRun` | `tekton.dev/v1/taskruns` | A single execution of a Task (child of PipelineRun) |

### 3.1 Listing PipelineRuns

```go
var pipelineRunGVR = schema.GroupVersionResource{
    Group:    "tekton.dev",
    Version:  "v1",
    Resource: "pipelineruns",
}

func (c *Client) ListPipelineRuns(ctx context.Context, limit int) ([]PipelineRunInfo, error) {
    list, err := c.dyn.Resource(pipelineRunGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{})
    // ...
}
```

This is equivalent to running:
```bash
kubectl get pipelineruns -n <namespace> -o json
```

The response is a list of unstructured JSON objects. We parse each one into our
`PipelineRunInfo` struct by navigating the JSON map:

```go
// Extract pipeline name from: spec.pipelineRef.name
spec["pipelineRef"].(map[string]interface{})["name"]

// Extract status from: status.conditions[-1].status
// "True" = Succeeded, "False" = Failed, "Unknown" = Running
conditions[-1]["status"]

// Extract timing from: status.startTime, status.completionTime
```

### 3.2 Listing TaskRuns for a PipelineRun

```go
func (c *Client) ListTaskRunsForPipeline(ctx context.Context, pipelineRunName string) ([]TaskRunInfo, error) {
    labelSelector := fmt.Sprintf("tekton.dev/pipelineRun=%s", pipelineRunName)
    list, err := c.dyn.Resource(taskRunGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{
        LabelSelector: labelSelector,
    })
}
```

Tekton automatically labels every TaskRun with `tekton.dev/pipelineRun=<name>`.
We use a **label selector** to fetch only TaskRuns belonging to a specific PipelineRun.

Equivalent to:
```bash
kubectl get taskruns -n <ns> -l tekton.dev/pipelineRun=my-pipeline-run-xyz
```

Each TaskRun also has the label `tekton.dev/pipelineTask` which contains the **task name**
(e.g., `create-release`, `git-clone`), which is what we display in the TUI.

### 3.3 Identifying Failed Tasks

```go
func (c *Client) GetFailedTaskRuns(ctx context.Context, pipelineRunName string) ([]TaskRunInfo, error) {
    tasks, err := c.ListTaskRunsForPipeline(ctx, pipelineRunName)
    // filter: keep only those where status == "Failed"
}
```

A task is "Failed" when its `status.conditions` has `status: "False"`. We also extract
the failure reason from `status.steps[*].terminated.message` for display.

---

## 4. How Retry/Rerun Triggers a New Execution

This is the core mechanism. **Tekton is declarative** — you don't "restart" a run.
Instead, you **create a brand new resource** and the Tekton controller reconciles it.

### 4.1 Retrying a Single Task (`buildRetryTaskRun`)

```go
func buildRetryTaskRun(original *unstructured.Unstructured) *unstructured.Unstructured {
    spec, _ := original.Object["spec"].(map[string]interface{})  // ①
    newName := safeRetryName(original.GetName())                  // ②

    newTaskRun := &unstructured.Unstructured{
        Object: map[string]interface{}{
            "apiVersion": "tekton.dev/v1",                        // ③
            "kind":       "TaskRun",
            "metadata": map[string]interface{}{
                "name":      newName,
                "namespace": original.GetNamespace(),
                "labels":    ...,                                 // ④
            },
            "spec": deepCopyJSON(spec),                           // ⑤
        },
    }
    return newTaskRun
}
```

**Step by step:**

| # | What | Why |
|---|------|-----|
| ① | Extract `spec` from the original failed TaskRun | The spec contains **everything** needed to run the task: taskRef, params, workspaces, serviceAccountName, etc. |
| ② | Generate a safe name (`safeRetryName`) | K8s names must be ≤63 chars. We truncate the base and append `-r-<timestamp>` |
| ③ | Set `apiVersion` and `kind` | Tells K8s this is a `tekton.dev/v1` `TaskRun` |
| ④ | Add traceability labels | `pipeline-retry/original-taskrun` links back to the original |
| ⑤ | Deep-copy the spec | `deepCopyJSON` does `Marshal → Unmarshal` to ensure a clean copy with no shared references |

**What happens after `Create()`:**

```go
created, err := c.dyn.Resource(taskRunGVR).Namespace(c.namespace).Create(ctx, newTaskRun, metav1.CreateOptions{})
```

```
                                    ┌──────────────────────────────┐
  CLI creates TaskRun YAML ────────▶│  K8s API Server              │
                                    │  (validates, stores in etcd) │
                                    └──────────┬───────────────────┘
                                               │
                                               ▼
                                    ┌──────────────────────────────┐
                                    │  Tekton Pipeline Controller  │
                                    │  (watches for new TaskRuns)  │
                                    │                              │
                                    │  1. Sees new TaskRun         │
                                    │  2. Resolves taskRef → Task  │
                                    │  3. Creates a Pod            │
                                    │  4. Pod runs step containers │
                                    │  5. Updates TaskRun status   │
                                    └──────────────────────────────┘
```

The Tekton controller is a **Kubernetes operator** that watches for `TaskRun` resources.
When we `Create()` a new one, the controller:

1. **Picks it up** via its watch/informer
2. **Resolves** the `taskRef` to find the actual `Task` definition
3. **Injects parameters and workspaces** from the spec
4. **Creates a Pod** with one container per step defined in the Task
5. **The Pod runs** on a cluster node, executing each step sequentially
6. **Updates `status.conditions`** on the TaskRun as it progresses

### 4.2 Re-running an Entire Pipeline (`buildRetryPipelineRun`)

Same concept, but at the PipelineRun level:

```go
func buildRetryPipelineRun(original *unstructured.Unstructured) *unstructured.Unstructured {
    spec, _ := original.Object["spec"].(map[string]interface{})
    // Build new PipelineRun with same spec + new name + traceability labels+annotations
}
```

The original PipelineRun spec contains:
```yaml
spec:
  pipelineRef:
    name: managed-ecom-release-pipeline   # which Pipeline to run
  params:                                  # input parameters
    - name: repo-url
      value: https://github.com/...
  workspaces:                              # shared storage
    - name: shared-workspace
      persistentVolumeClaim:
        claimName: my-pvc
  serviceAccountName: tekton-sa            # RBAC identity
```

When we copy this spec into a new PipelineRun and `Create()` it:

1. Tekton controller resolves `pipelineRef` → finds the `Pipeline` resource
2. For each task in the Pipeline's `tasks[]` list, creates a **child TaskRun**
3. Respects `runAfter`, `when` conditions, and parallel execution
4. Each child TaskRun follows the flow described in §4.1

### 4.3 Name Generation (`safeRetryName`)

```go
func safeRetryName(baseName string) string {
    suffix := fmt.Sprintf("-r-%d", time.Now().Unix()%1000000)  // e.g., "-r-482156"
    maxBase := 63 - len(suffix)
    if len(baseName) > maxBase {
        baseName = baseName[:maxBase]
    }
    baseName = strings.TrimRight(baseName, "-")
    return baseName + suffix
}
```

Example:
```
Original:  managed-ecom-release-pipeline-hjv972-create-release-retry  (62+ chars)
Generated: managed-ecom-release-pipeline-hjv972-create-releas-r-482156  (≤63 chars)
```

---

## 5. The TUI Layer

### 5.1 Bubbletea Architecture

[Bubbletea](https://github.com/charmbracelet/bubbletea) uses the **Elm Architecture**:

```
         ┌─────────────┐
         │   Model     │  ← State (cursor position, selections, search text)
         └──────┬──────┘
                │
    ┌───────────▼───────────┐
    │   Update(msg) → Model │  ← Handle keyboard/resize events, return new state
    └───────────┬───────────┘
                │
    ┌───────────▼───────────┐
    │   View() → string     │  ← Render current state to terminal string
    └───────────────────────┘
```

We have 3 TUI models:

| Model | File | Purpose |
|-------|------|---------|
| `PipelineRunListModel` | `tui/list.go` | Scrollable list of PipelineRuns |
| `TaskSelectorModel` | `tui/selector.go` | Fuzzy search + multi-select for tasks |
| `ConfirmModel` | `tui/confirm.go` | Yes/No confirmation dialog |

### 5.2 Fuzzy Search (Task Selector)

```go
func fuzzyMatch(text, pattern string) bool {
    // 1. Try substring match first
    if strings.Contains(text, pattern) {
        return true
    }
    // 2. Fuzzy: all pattern chars must appear in order
    pi := 0
    for ti := 0; ti < len(text) && pi < len(pattern); ti++ {
        if text[ti] == pattern[pi] {
            pi++
        }
    }
    return pi == len(pattern)
}
```

Typing `"cr"` matches `"create-release"` (substring) and `"archive-logs"` (fuzzy: **c**→a**r**chive).

The search runs on every keystroke via `filterTasks()` which re-filters the full task list.

### 5.3 Selection Flow

```
User types in search box
        │
        ▼
filterTasks() runs fuzzy match on all tasks
        │
        ▼
View() renders only matching tasks with cursor
        │
        ▼
Space/Tab toggles selection (●/○)
        │
        ▼
Enter auto-selects cursor item if nothing selected, then confirms
        │
        ▼
SelectedTasks() returns []string of task names to retry
```

---

## 6. End-to-End Flow: `pipeline-retry retry`

```
┌─ User runs: pipeline-retry retry -n my-ns my-pipeline-run-xyz ──────────────┐
│                                                                               │
│  1. NewClient("my-ns")                                                        │
│     └─ Loads ~/.kube/config → REST config → dynamic.Client                    │
│                                                                               │
│  2. GetFailedTaskRuns("my-pipeline-run-xyz")                                  │
│     └─ GET /apis/tekton.dev/v1/namespaces/my-ns/taskruns                      │
│        ?labelSelector=tekton.dev/pipelineRun=my-pipeline-run-xyz              │
│     └─ Filter: keep only status.conditions[-1].status == "False"              │
│                                                                               │
│  3. TUI: TaskSelectorModel (fuzzy search + multi-select)                      │
│     └─ User sees: ✗ create-release  ✗ archive-logs                            │
│     └─ User selects: create-release                                           │
│                                                                               │
│  4. TUI: ConfirmModel ("Retry 1 task(s)?")                                    │
│     └─ User presses: Yes                                                      │
│                                                                               │
│  5. RetryFailedTasks("my-pipeline-run-xyz", ["create-release"])               │
│     ├─ GET original TaskRun (full spec)                                       │
│     ├─ buildRetryTaskRun(original)                                            │
│     │   ├─ Deep-copy spec (taskRef, params, workspaces, etc.)                 │
│     │   ├─ Generate safe name (≤63 chars)                                     │
│     │   └─ Add traceability labels                                            │
│     └─ POST /apis/tekton.dev/v1/namespaces/my-ns/taskruns                     │
│        └─ Tekton controller creates Pod → runs task → updates status          │
│                                                                               │
│  6. Output: "OK create-release -> my-pipeline-run-xyz-create-r-482156"        │
└───────────────────────────────────────────────────────────────────────────────┘
```

---

## 7. Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Dynamic K8s client** instead of typed Tekton SDK | Avoids ~50+ transitive Go module dependencies. We only read a few fields. |
| **Copy full `spec`** for retry | Preserves all params, workspaces, serviceAccount, nodeSelector, tolerations, etc. No information loss. |
| **Create new resource** instead of patching | Tekton is immutable — completed Runs can't be restarted. Creating a new one is the supported pattern. |
| **Label-based TaskRun discovery** | Uses Tekton's built-in `tekton.dev/pipelineRun` label. No need to parse PipelineRun status internals. |
| **`safeRetryName` with truncation** | K8s enforces a 63-char limit on resource names. Original TaskRun names from Tekton can already be 50+ chars. |
| **Fuzzy search** instead of exact match | Pipeline runs can have many tasks. Fuzzy narrows the list quickly without requiring exact names. |
| **`deepCopyJSON` via Marshal/Unmarshal** | Clean way to deep-copy `map[string]interface{}` without shared pointers. Ensures the new resource has independent data. |
