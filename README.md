# KSTool

![image](KSTool.png)

A Kubernetes job management tool with a terminal-based user interface.

## Features

- 🖥️ Terminal-based UI with intuitive keyboard controls
- 📊 Real-time job status monitoring
- 🔍 Advanced filtering and sorting capabilities
- 🎨 Color-coded GPU and status information
- ⚙️ Job configuration management
- 🔒 User-specific job access control

## Installation

```bash
git clone https://github.com/Suchun-sv/KSTool.git
cd KSTool
go mod tidy
go build
# if you meet some compatibility issues, you can try to use the following command to install the dependencies
# CGO_ENABLED=0 go build -o kstool main.go
```

## Usage

```bash
./kstool
```

The main interface provides a comprehensive view of all Kubernetes jobs with the following information:
- Job Name
- Status (Running/Complete/Failed/Pending)
- Completions
- Duration
- Age
- Number of Pods
- GPU Count
- GPU Information (Type and Memory)

### Keyboard Controls

#### Main Interface
- `r` - Refresh job list
- `f` - Cycle through status filters (All/Running/Failed/Suspended)
- `h` - Toggle user filter (Show all/Show only your jobs)
- `s` - Cycle through sort modes:
  - Age (Descending/Ascending)
  - GPU Count (Descending/Ascending)
  - Duration (Descending/Ascending)
  - GPU Type (Descending/Ascending)
- `d` - Delete selected job (only your own jobs)
- `e` - Enter selected job's pod (only for running jobs)
- `n` - Create new job configuration
- `Ctrl+C` - Exit application

#### Configuration Management
- `l` - Load selected configuration
- `d` - Delete selected configuration
- `n` - Create new configuration
- `q` - Exit configuration list

#### Configuration Form
- `j` - Move to next field
- `k` - Move to previous field
- `F5` - Apply configuration
- `Ctrl+S` - Save configuration
- `Esc` - Return to configuration list

### GPU Support

The tool supports various GPU types with color-coded display:
- H200 (Gold)
- H100 (Purple)
- A100 (Blue)
- No GPU (Gray)

GPU information includes:
- Type (H200/H100/A100)
- Memory (40G/80G)
- Status indicator (⌛️ for pending jobs)

### Job Configuration

Create and manage job configurations with the following parameters:
- User
- Queue Name
- Image Name
- Command
- CPU Number
- Memory
- GPU Number
- GPU Product
- Mount Path
- Workspace PVC
- NFS Path
- NFS Server

### Security Features

- User-specific job access control
- Only allow deletion of user's own jobs
- Configuration files stored in user's home directory

## Development

### Building

```bash
# Build with CGO disabled
CGO_ENABLED=0 go build
```

### VSCode Tasks

#### Configuration List
- `l`: Load selected configuration
- `d`: Delete selected configuration
- `n`: Create new configuration
- `q`: Exit configuration list

## License

MIT License