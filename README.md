# KSTool

![image](KSTool.png)

A Kubernetes job management tool with a terminal-based user interface.

## Features

- View Kubernetes jobs with detailed information
- Color-coded GPU information display
- Interactive job deletion with confirmation
- Real-time status updates

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
```

## Usage

```bash
./kstool
```

### Keyboard Controls

- `d`: Delete selected job
- `Ctrl+C`: Exit the application

## License

MIT