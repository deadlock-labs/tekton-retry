# pipeline-retry

An interactive Go CLI/TUI tool for retrying failed **Tekton** pipeline tasks with fuzzy search and multi-selection.

## Features

| Feature | Description |
|---|---|
| **List Pipeline Runs** | Browse recent PipelineRuns in a scrollable, color-coded list |
| **Fuzzy Search** | Type to filter failed tasks by name, status, or failure reason |
| **Multi-Select** | Pick specific failed tasks to retry (space/tab to toggle) |
| **Retry All** | Retry every failed task in a pipeline run at once |
| **Re-run** | Re-run an entire PipelineRun from scratch |
| **Confirmation** | Interactive yes/no confirmation before making changes |

## Prerequisites

- Go 1.21+
- Access to a Kubernetes cluster with Tekton Pipelines installed
- Valid `kubeconfig` configured (same as `kubectl`)

## Installation

### Build from source

```bash
make build
./bin/pipeline-retry --help

# Or install to $GOPATH/bin
make install
pipeline-retry --help
```

### Use pre-built binaries globally

Download the appropriate binary from the `bin/` directory for your platform:

| Platform | Binary |
|---|---|
| macOS (Apple Silicon) | `pipeline-retry-darwin-arm64` |
| macOS (Intel) | `pipeline-retry-darwin-amd64` |
| Windows (x86_64) | `pipeline-retry-windows-amd64.exe` |

#### macOS / Linux

```bash
# Copy the binary to a directory in your PATH
sudo cp bin/pipeline-retry-darwin-arm64 /usr/local/bin/pipeline-retry

# Make it executable
sudo chmod +x /usr/local/bin/pipeline-retry

# Verify
pipeline-retry --help
```

#### Windows

```powershell
# Copy the binary to a directory in your PATH (e.g., C:\Users\<you>\bin)
copy bin\pipeline-retry-windows-amd64.exe C:\Users\%USERNAME%\bin\pipeline-retry.exe

# Add to PATH if not already (run once in PowerShell as Admin)
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\Users\$env:USERNAME\bin", "User")

# Restart your terminal, then verify
pipeline-retry --help
```

## Usage

### List recent pipeline runs
```bash
pipeline-retry list                     # uses current namespace
pipeline-retry list -n my-namespace     # specify namespace
pipeline-retry list -l 50              # show up to 50 runs
```

### Retry failed tasks interactively
```bash
# Interactive: select a pipeline run, then fuzzy-search failed tasks
pipeline-retry retry

# Direct: specify the pipeline run name
pipeline-retry retry my-pipeline-run-abc123

# With a specific namespace
pipeline-retry retry -n ci-namespace my-pipeline-run-abc123
```

### Retry ALL failed tasks at once
```bash
pipeline-retry retry-all                          # interactive selection
pipeline-retry retry-all my-pipeline-run-abc123   # direct
```

### Re-run an entire pipeline
```bash
pipeline-retry rerun my-pipeline-run-abc123
```

## Interactive Controls

### Pipeline Run List
| Key | Action |
|---|---|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `Enter` | Select |
| `q` / `Esc` | Quit |

### Task Selector (Fuzzy Search)
| Key | Action |
|---|---|
| Type | Filter tasks |
| `↑` / `↓` | Navigate |
| `Space` / `Tab` | Toggle selection |
| `Ctrl+A` | Select/deselect all |
| `Enter` | Confirm selection |
| `Esc` | Clear filter / go back |

### Confirmation Dialog
| Key | Action |
|---|---|
| `←` / `→` | Switch Yes/No |
| `y` / `n` | Quick select |
| `Enter` | Confirm |

## How It Works

1. Connects to your Kubernetes cluster using your current kubeconfig
2. Queries Tekton CRDs (`PipelineRun`, `TaskRun`) via the dynamic K8s client
3. Presents results in an interactive Bubbletea TUI with Lipgloss styling
4. To retry a task, it creates a **new TaskRun** based on the failed one's spec
5. To re-run a pipeline, it creates a **new PipelineRun** based on the original spec

## Architecture

```
pipeline-retry/
├── main.go                      # Entry point
├── cmd/
│   ├── root.go                  # Root command + global flags
│   ├── list.go                  # `list` subcommand
│   ├── retry.go                 # `retry` subcommand (interactive select)
│   ├── retry_all.go             # `retry-all` subcommand
│   └── rerun.go                 # `rerun` subcommand
├── internal/
│   ├── tekton/
│   │   ├── client.go            # K8s dynamic client for Tekton CRDs
│   │   └── types.go             # Data types (PipelineRunInfo, TaskRunInfo)
│   └── tui/
│       ├── list.go              # Pipeline run list TUI component
│       ├── selector.go          # Fuzzy search + multi-select TUI
│       ├── confirm.go           # Confirmation dialog TUI
│       ├── styles.go            # Lipgloss color/style definitions
│       └── helpers.go           # Utility functions
├── Makefile
└── README.md
```

## Dependencies

- [cobra](https://github.com/spf13/cobra) — CLI framework
- [bubbletea](https://github.com/charmbracelet/bubbletea) — Terminal UI framework
- [lipgloss](https://github.com/charmbracelet/lipgloss) — Terminal styling
- [client-go](https://github.com/kubernetes/client-go) — Kubernetes Go client

## Roadmap

- **Web-based integration** — A browser-based UI for managing and retrying Tekton pipeline tasks is under development and will be available soon.
