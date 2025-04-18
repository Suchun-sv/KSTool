# KSTool

![image](KSTool.png)

A Kubernetes job management tool with a terminal-based user interface.

## Features

- View Kubernetes jobs with detailed information
- Color-coded GPU information display
- Interactive job deletion with confirmation
- Real-time status updates
- Job configuration management
  - Create and save job configurations
  - Load existing configurations
  - Delete configurations with confirmation
  - Support for multiple GPU types (H200, H100, A100)

## Requirements

- Go 1.16 or later
- kubectl configured with access to your Kubernetes cluster
- tview library (`go get github.com/rivo/tview`)

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


### Keyboard Controls

#### Main Interface
- `d`: Delete selected job
- `Ctrl+C`: Exit the application

#### Configuration Form
- `Ctrl+S` (or `Cmd+S` on macOS): Save current configuration
- `F5`: Apply configuration to create a job
- `Esc`: Return to configuration list
- `Up/Down`: Navigate between form fields
- `Enter`: Save configuration name

#### Configuration List
- `l`: Load selected configuration
- `d`: Delete selected configuration
- `n`: Create new configuration
- `q`: Exit configuration list

## License

MIT