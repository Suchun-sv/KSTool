# KSTool

KSTool is a terminal application for managing Kubernetes jobs on EIDF clusters. It lets you monitor jobs, create new ones from templates and perform common maintenance actions without leaving the command line.

## Features
- **Job overview** with name, status, completions, runtime, age, pod count and GPU information.
- **Filtering** by job status (all, running, failed or pending) and by current user.
- **Sorting** by age, GPU count, runtime or GPU type.
- **Actions**: delete a job (with confirmation), open a shell inside a job's pod and refresh the list.
- **Configuration menu** for creating jobs from a YAML template with environment variables. Values can be edited in a form or in Vim, saved for reuse and applied directly.
- **Colorful interface** and keyboard shortcuts (`r`, `d`, `e`, `f`, `h`, `s`, `n`, `q`).

## Quick start
1. **Download a release**
   ```bash
   wget https://github.com/Suchun-sv/KSTool/releases/latest/download/kstool
   chmod +x kstool
   ./kstool
   ```
2. **Build from source** (requires Go and kubectl)
   ```bash
   git clone https://github.com/Suchun-sv/KSTool.git
   cd KSTool
   go build
   ```

## Creating jobs
Job definitions use environment variables in the form `${VAR:-default}`. Copy `config/base_apply.yaml` to `~/.kstool/base_apply.yaml` and edit it or start from the examples in `config/examples/`. Press `n` in the program to open the configuration menu and fill in values. Saved configs are stored in `~/.kstool/env_config_list/`.

## Requirements
- Go 1.21+
- kubectl configured for your cluster
- envsubst
- Vim for advanced editing

KSTool is released under the MIT License.
