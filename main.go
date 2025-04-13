package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Constants
const (
	NAMESPACE = "eidf029ns"
	APP_NAME  = "KSTool"
	VERSION   = "0.1.0"
	AUTHOR    = "suchun"
)

// Emoji constants
const (
	EMOJI_WAITING = "⏳"
	EMOJI_WARNING = "⚠️"
)

// Color constants
const (
	COLOR_HEADER    = tcell.ColorWhite
	COLOR_RUNNING   = tcell.ColorGreen
	COLOR_COMPLETE  = tcell.ColorBlue
	COLOR_FAILED    = tcell.ColorRed
	COLOR_SUSPENDED = tcell.ColorYellow
	COLOR_WAITING   = tcell.ColorGray
	COLOR_H200      = tcell.ColorGold
	COLOR_H100      = tcell.ColorPurple
	COLOR_A100      = tcell.ColorBlue
	COLOR_NO_GPU    = tcell.ColorGray
	COLOR_DEFAULT   = tcell.ColorWhite
)

// Job represents a Kubernetes job with its details
type Job struct {
	Name        string
	Status      string
	Completions string
	Duration    string
	Age         string
	Pods        string
	GPUInfo     string
}

// getJobPods retrieves the pods associated with a job
func getJobPods(jobName string) (string, error) {
	cmd := exec.Command("kubectl", "get", "pods", "-n", NAMESPACE, "-l", fmt.Sprintf("job-name=%s", jobName))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get pods: %w", err)
	}
	return string(output), nil
}

// getJobGPUInfo retrieves GPU information for a job
func getJobGPUInfo(jobName string) (string, error) {
	// First get the pod name
	podsCmd := exec.Command("kubectl", "get", "pods", "-n", NAMESPACE, "-l", fmt.Sprintf("job-name=%s", jobName))
	podsOutput, err := podsCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get pods: %w", err)
	}

	podLines := strings.Split(string(podsOutput), "\n")
	var output []byte
	var prefix string

	if len(podLines) < 2 {
		// If no pod found, get GPU info from job description
		describeCmd := exec.Command("kubectl", "describe", "job", jobName, "-n", NAMESPACE)
		output, err = describeCmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to describe job: %w", err)
		}
		prefix = EMOJI_WAITING + " "
	} else {
		podName := strings.Fields(podLines[1])[0]
		describeCmd := exec.Command("kubectl", "describe", "pod", podName, "-n", NAMESPACE)
		output, err = describeCmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to describe pod: %w", err)
		}
	}

	// Parse GPU information
	gpuInfo := parseGPUInfo(string(output))
	if gpuInfo == "" {
		return prefix + "No GPU", nil
	}
	return prefix + gpuInfo, nil
}

// parseGPUInfo extracts GPU information from describe output
func parseGPUInfo(output string) string {
	lines := strings.Split(output, "\n")
	var gpuCount, gpuModel, gpuMemory string

	for _, line := range lines {
		if strings.Contains(line, "nvidia.com/gpu:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				gpuCount = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "nvidia.com/gpu.product=") {
			parts := strings.Split(line, "=")
			if len(parts) > 1 {
				model := strings.TrimSpace(parts[1])
				gpuModel = extractGPUModel(model)
				gpuMemory = extractGPUMemory(model)
			}
		}
	}

	if gpuCount == "" {
		return ""
	}

	if gpuModel == "" {
		return fmt.Sprintf("%s GPU", gpuCount)
	}

	if gpuMemory != "" {
		return fmt.Sprintf("%s %s %s", gpuCount, gpuModel, gpuMemory)
	}

	return fmt.Sprintf("%s %s", gpuCount, gpuModel)
}

// extractGPUModel extracts the GPU model from the full model string
func extractGPUModel(model string) string {
	model = strings.ToUpper(model)
	switch {
	case strings.Contains(model, "A100"):
		return "A100"
	case strings.Contains(model, "H100"):
		return "H100"
	case strings.Contains(model, "H200"):
		return "H200"
	default:
		return model
	}
}

// extractGPUMemory extracts the GPU memory from the model string
func extractGPUMemory(model string) string {
	switch {
	case strings.Contains(model, "80GB"):
		return "80G"
	case strings.Contains(model, "40GB"):
		return "40G"
	case strings.Contains(model, "24GB"):
		return "24G"
	case strings.Contains(model, "16GB"):
		return "16G"
	case strings.Contains(model, "12GB"):
		return "12G"
	case strings.Contains(model, "8GB"):
		return "8G"
	case strings.Contains(model, "6GB"):
		return "6G"
	default:
		return ""
	}
}

// getJobs retrieves all jobs in the namespace
func getJobs() ([]Job, error) {
	cmd := exec.Command("kubectl", "get", "jobs", "-n", NAMESPACE)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	var jobs []Job

	// Skip header line
	for i := 1; i < len(lines); i++ {
		line := strings.Fields(lines[i])
		if len(line) < 1 {
			continue
		}

		jobName := line[0]

		// Get pod information
		pods, _ := getJobPods(jobName)
		podLines := strings.Split(pods, "\n")
		podCount := len(podLines) - 1 // Subtract 1 for header line

		// Get GPU information
		gpuInfo, _ := getJobGPUInfo(jobName)

		// Get fields with default values
		getField := func(index int) string {
			if len(line) > index {
				return line[index]
			}
			return "N/A"
		}

		jobs = append(jobs, Job{
			Name:        jobName,
			Status:      getField(1),
			Completions: getField(2),
			Duration:    getField(3),
			Age:         getField(4),
			Pods:        fmt.Sprintf("%d pods", podCount),
			GPUInfo:     gpuInfo,
		})
	}

	return jobs, nil
}

// deleteJob deletes a job by name
func deleteJob(jobName string) error {
	cmd := exec.Command("kubectl", "delete", "jobs", jobName, "-n", NAMESPACE)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// getStatusColor returns the color for a job status
func getStatusColor(status string) tcell.Color {
	switch status {
	case "Running":
		return COLOR_RUNNING
	case "Complete":
		return COLOR_COMPLETE
	case "Failed":
		return COLOR_FAILED
	case "Suspended":
		return COLOR_SUSPENDED
	default:
		return COLOR_DEFAULT
	}
}

// getGPUColor returns the color for GPU information
func getGPUColor(gpuInfo string) tcell.Color {
	switch {
	case strings.Contains(gpuInfo, EMOJI_WAITING):
		return COLOR_WAITING
	case strings.Contains(gpuInfo, "H200"):
		return COLOR_H200
	case strings.Contains(gpuInfo, "H100"):
		return COLOR_H100
	case strings.Contains(gpuInfo, "A100"):
		return COLOR_A100
	case strings.Contains(gpuInfo, "No GPU"):
		return COLOR_NO_GPU
	default:
		return COLOR_DEFAULT
	}
}

// createTable creates and configures the main table
func createTable() *tview.Table {
	table := tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false)

	// Set headers
	headers := []string{"NAME", "STATUS", "COMPLETIONS", "DURATION", "AGE", "PODS", "GPU INFO"}
	for i, header := range headers {
		table.SetCell(0, i, tview.NewTableCell(header).
			SetTextColor(COLOR_HEADER).
			SetAlign(tview.AlignCenter).
			SetSelectable(false))
	}

	return table
}

// updateTable updates the table with job information
func updateTable(table *tview.Table, jobs []Job) {
	// Clear existing rows except header
	for i := table.GetRowCount() - 1; i > 0; i-- {
		table.RemoveRow(i)
	}

	// Add new jobs
	for i, job := range jobs {
		statusColor := getStatusColor(job.Status)
		gpuColor := getGPUColor(job.GPUInfo)

		table.SetCell(i+1, 0, tview.NewTableCell(job.Name))
		table.SetCell(i+1, 1, tview.NewTableCell(job.Status).SetTextColor(statusColor))
		table.SetCell(i+1, 2, tview.NewTableCell(job.Completions))
		table.SetCell(i+1, 3, tview.NewTableCell(job.Duration))
		table.SetCell(i+1, 4, tview.NewTableCell(job.Age))
		table.SetCell(i+1, 5, tview.NewTableCell(job.Pods))
		table.SetCell(i+1, 6, tview.NewTableCell(job.GPUInfo).SetTextColor(gpuColor))
	}
}

// createASCIIArt creates the ASCII art header
func createASCIIArt() *tview.TextView {
	return tview.NewTextView().
		SetTextAlign(tview.AlignLeft).
		SetText(fmt.Sprintf(`
 ██╗  ██╗███████╗████████╗ ██████╗  ██████╗ ██╗     
 ██║ ██╔╝██╔════╝╚══██╔══╝██╔═══██╗██╔═══██╗██║     
 ██████╔╝███████╗   ██║   ██║   ██║██║   ██║██║     
 ██╔═██╗ ╚════██║   ██║   ██║   ██║██║   ██║██║     
 ██║  ██╗███████║   ██║   ╚██████╔╝╚██████╔╝███████╗
 ╚═╝  ╚═╝╚══════╝   ╚═╝    ╚═════╝  ╚═════╝ ╚══════╝
===================================================
(d)elete (r)efresh (ctrl+c)exit

`)).
		SetTextColor(COLOR_A100)
}

// createVersionInfo creates the version info footer
func createVersionInfo() *tview.TextView {
	return tview.NewTextView().
		SetTextAlign(tview.AlignLeft).
		SetText(fmt.Sprintf("%s@%s by %s", APP_NAME, VERSION, AUTHOR)).
		SetTextColor(COLOR_DEFAULT)
}

// createDeleteModal creates the delete confirmation modal
func createDeleteModal(app *tview.Application, flex *tview.Flex, jobName, jobStatus string, onConfirm func()) *tview.Flex {
	warningText := fmt.Sprintf("%s WARNING: You are about to delete job '%s'\n\n"+
		"Current Status: %s\n"+
		"Namespace: %s\n\n"+
		"This action cannot be undone!\n\n"+
		"Type 'DELETE' to confirm deletion:",
		EMOJI_WARNING, jobName, jobStatus, NAMESPACE)

	inputField := tview.NewInputField().
		SetLabel("Confirmation: ").
		SetFieldWidth(20)

	modal := tview.NewModal().
		SetText(warningText).
		AddButtons([]string{"Cancel"})

	modalFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(modal, 0, 1, true).
		AddItem(inputField, 1, 0, true)

	inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			if inputField.GetText() == "DELETE" {
				onConfirm()
			} else {
				app.SetRoot(flex, true)
			}
		}
	})

	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		app.SetRoot(flex, true)
	})

	return modalFlex
}

func main() {
	jobs, err := getJobs()
	if err != nil {
		panic(err)
	}

	app := tview.NewApplication()

	// Create main layout
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow)

	// Add components
	flex.AddItem(createASCIIArt(), 7, 0, false)
	table := createTable()
	flex.AddItem(table, 0, 1, true)
	flex.AddItem(createVersionInfo(), 1, 0, false)

	// Initial table update
	updateTable(table, jobs)

	// Set up keyboard navigation
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			app.Stop()
		case tcell.KeyRune:
			switch event.Rune() {
			case 'd':
				row, _ := table.GetSelection()
				if row > 0 {
					jobName := table.GetCell(row, 0).Text
					jobStatus := table.GetCell(row, 1).Text

					modalFlex := createDeleteModal(app, flex, jobName, jobStatus, func() {
						// Show processing message
						processingModal := tview.NewModal().
							SetText(fmt.Sprintf("Deleting job %s...", jobName)).
							AddButtons([]string{"OK"})
						app.SetRoot(processingModal, false)

						// Perform deletion
						if err := deleteJob(jobName); err != nil {
							errorModal := tview.NewModal().
								SetText(fmt.Sprintf("Error deleting job: %v", err)).
								AddButtons([]string{"OK"}).
								SetDoneFunc(func(buttonIndex int, buttonLabel string) {
									app.SetRoot(flex, true)
								})
							app.SetRoot(errorModal, false)
						} else {
							// Show success message and refresh
							successModal := tview.NewModal().
								SetText(fmt.Sprintf("Successfully deleted job %s", jobName)).
								AddButtons([]string{"OK"}).
								SetDoneFunc(func(buttonIndex int, buttonLabel string) {
									newJobs, err := getJobs()
									if err == nil {
										jobs = newJobs
										updateTable(table, jobs)
									}
									app.SetRoot(flex, true)
								})
							app.SetRoot(successModal, false)
						}
					})

					app.SetRoot(modalFlex, true)
					app.SetFocus(modalFlex.GetItem(1))
				}
			case 'r':
				// Show refreshing message
				refreshingModal := tview.NewModal().
					SetText("Refreshing jobs list...").
					AddButtons([]string{"OK"})
				app.SetRoot(refreshingModal, false)

				// Refresh jobs list
				newJobs, err := getJobs()
				if err == nil {
					jobs = newJobs
					updateTable(table, jobs)
				}
				app.SetRoot(flex, true)
			}
		}
		return event
	})

	if err := app.SetRoot(flex, true).SetFocus(table).Run(); err != nil {
		panic(err)
	}
}
