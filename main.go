package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Job struct {
	Name        string
	Status      string
	Completions string
	Duration    string
	Age         string
	Pods        string
	GPUInfo     string
}

func getJobPods(jobName string) (string, error) {
	cmd := exec.Command("kubectl", "get", "pods", "-n", "eidf029ns", "-l", fmt.Sprintf("job-name=%s", jobName))
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func getJobGPUInfo(jobName string) (string, error) {
	// First get the pod name
	podsCmd := exec.Command("kubectl", "get", "pods", "-n", "eidf029ns", "-l", fmt.Sprintf("job-name=%s", jobName))
	podsOutput, err := podsCmd.Output()
	if err != nil {
		return "", err
	}

	podLines := strings.Split(string(podsOutput), "\n")
	if len(podLines) < 2 {
		return "No pods", nil
	}

	podName := strings.Fields(podLines[1])[0]

	// Get GPU information from pod
	describeCmd := exec.Command("kubectl", "describe", "pod", podName, "-n", "eidf029ns")
	output, err := describeCmd.Output()
	if err != nil {
		return "", err
	}

	// Parse GPU information from describe output
	lines := strings.Split(string(output), "\n")
	var gpuCount string
	var gpuModel string
	var gpuMemory string
	for _, line := range lines {
		if strings.Contains(line, "nvidia.com/gpu:") {
			// Extract GPU count
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				gpuCount = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "nvidia.com/gpu.product=") {
			// Extract GPU model and memory
			parts := strings.Split(line, "=")
			if len(parts) > 1 {
				model := strings.TrimSpace(parts[1])
				// Extract GPU model
				if strings.Contains(strings.ToUpper(model), "A100") {
					gpuModel = "A100"
				} else if strings.Contains(strings.ToUpper(model), "H100") {
					gpuModel = "H100"
				} else if strings.Contains(strings.ToUpper(model), "H200") {
					gpuModel = "H200"
				} else {
					gpuModel = model
				}
				// Extract GPU memory
				if strings.Contains(model, "80GB") {
					gpuMemory = "80G"
				} else if strings.Contains(model, "40GB") {
					gpuMemory = "40G"
				} else if strings.Contains(model, "24GB") {
					gpuMemory = "24G"
				} else if strings.Contains(model, "16GB") {
					gpuMemory = "16G"
				} else if strings.Contains(model, "12GB") {
					gpuMemory = "12G"
				} else if strings.Contains(model, "8GB") {
					gpuMemory = "8G"
				} else if strings.Contains(model, "6GB") {
					gpuMemory = "6G"
				}
			}
		}
	}

	if gpuCount == "" {
		return "No GPU", nil
	}

	if gpuModel == "" {
		return fmt.Sprintf("%s GPU", gpuCount), nil
	}

	if gpuMemory != "" {
		return fmt.Sprintf("%s %s %s", gpuCount, gpuModel, gpuMemory), nil
	}

	return fmt.Sprintf("%s %s", gpuCount, gpuModel), nil
}

func getJobs() ([]Job, error) {
	cmd := exec.Command("kubectl", "get", "jobs")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	var jobs []Job

	// Skip header line
	for i := 1; i < len(lines); i++ {
		line := strings.Fields(lines[i])
		if len(line) >= 5 {
			jobName := line[0]

			// Get pod information
			pods, _ := getJobPods(jobName)
			podLines := strings.Split(pods, "\n")
			podCount := len(podLines) - 1 // Subtract 1 for header line

			// Get GPU information
			gpuInfo, _ := getJobGPUInfo(jobName)

			jobs = append(jobs, Job{
				Name:        jobName,
				Status:      line[1],
				Completions: line[2],
				Duration:    line[3],
				Age:         line[4],
				Pods:        fmt.Sprintf("%d pods", podCount),
				GPUInfo:     gpuInfo,
			})
		}
	}

	return jobs, nil
}

func deleteJob(jobName string) error {
	cmd := exec.Command("kubectl", "delete", "jobs", jobName, "-n", "eidf029ns")

	// Set stdin, stdout, and stderr to the terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command and wait for it to complete
	return cmd.Run()
}

func main() {
	jobs, err := getJobs()
	if err != nil {
		panic(err)
	}

	app := tview.NewApplication()

	// Create a flex container to hold the ASCII art, table, and version info
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow)

	// Add ASCII art at the top
	asciiArt := tview.NewTextView().
		SetTextAlign(tview.AlignLeft).
		SetText(`
 ██╗  ██╗███████╗████████╗ ██████╗  ██████╗ ██╗     
 ██║ ██╔╝██╔════╝╚══██╔══╝██╔═══██╗██╔═══██╗██║     
 ██████╔╝███████╗   ██║   ██║   ██║██║   ██║██║     
 ██╔═██╗ ╚════██║   ██║   ██║   ██║██║   ██║██║     
 ██║  ██╗███████║   ██║   ╚██████╔╝╚██████╔╝███████╗
 ╚═╝  ╚═╝╚══════╝   ╚═╝    ╚═════╝  ╚═════╝ ╚══════╝
`).
		SetTextColor(tcell.ColorBlue)

	// Create the table
	table := tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false) // Enable row selection

	// Add version info at the bottom
	versionInfo := tview.NewTextView().
		SetTextAlign(tview.AlignLeft).
		SetText("KSTool@0.1.0 by suchun").
		SetTextColor(tcell.ColorGray)

	// Add all components to the flex container
	flex.AddItem(asciiArt, 7, 0, false)    // ASCII art takes 7 lines
	flex.AddItem(table, 0, 1, true)        // Table takes remaining space
	flex.AddItem(versionInfo, 1, 0, false) // Version info takes 1 line

	// Set headers
	headers := []string{"NAME", "STATUS", "COMPLETIONS", "DURATION", "AGE", "PODS", "GPU INFO"}
	for i, header := range headers {
		table.SetCell(0, i, tview.NewTableCell(header).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignCenter).
			SetSelectable(false))
	}

	// Function to update table with jobs
	updateTable := func(jobs []Job) {
		// Clear existing rows except header
		for i := table.GetRowCount() - 1; i > 0; i-- {
			table.RemoveRow(i)
		}
		// Add new jobs
		for i, job := range jobs {
			var statusColor tcell.Color
			switch job.Status {
			case "Running":
				statusColor = tcell.ColorGreen
			case "Complete":
				statusColor = tcell.ColorBlue
			case "Failed":
				statusColor = tcell.ColorRed
			case "Suspended":
				statusColor = tcell.ColorYellow
			default:
				statusColor = tcell.ColorWhite
			}

			// Set different colors based on GPU model
			var gpuColor tcell.Color
			if strings.Contains(job.GPUInfo, "H200") {
				gpuColor = tcell.ColorGold
			} else if strings.Contains(job.GPUInfo, "H100") {
				gpuColor = tcell.ColorPurple
			} else if strings.Contains(job.GPUInfo, "A100") {
				gpuColor = tcell.ColorBlue
			} else if strings.Contains(job.GPUInfo, "No GPU") {
				gpuColor = tcell.ColorGray
			} else {
				gpuColor = tcell.ColorWhite
			}

			table.SetCell(i+1, 0, tview.NewTableCell(job.Name))
			table.SetCell(i+1, 1, tview.NewTableCell(job.Status).SetTextColor(statusColor))
			table.SetCell(i+1, 2, tview.NewTableCell(job.Completions))
			table.SetCell(i+1, 3, tview.NewTableCell(job.Duration))
			table.SetCell(i+1, 4, tview.NewTableCell(job.Age))
			table.SetCell(i+1, 5, tview.NewTableCell(job.Pods))
			table.SetCell(i+1, 6, tview.NewTableCell(job.GPUInfo).SetTextColor(gpuColor))
		}
	}

	// Initial table update
	updateTable(jobs)

	// Add keyboard navigation
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			app.Stop()
		case tcell.KeyRune:
			switch event.Rune() {
			case 'd':
				row, _ := table.GetSelection()
				if row > 0 { // Skip header row
					jobName := table.GetCell(row, 0).Text
					jobStatus := table.GetCell(row, 1).Text

					warningText := fmt.Sprintf("⚠️ WARNING: You are about to delete job '%s'\n\n"+
						"Current Status: %s\n"+
						"Namespace: eidf029ns\n\n"+
						"This action cannot be undone!\n\n"+
						"Type 'DELETE' to confirm deletion:",
						jobName, jobStatus)

					// Create input field for confirmation
					inputField := tview.NewInputField().
						SetLabel("Confirmation: ").
						SetFieldWidth(20).
						SetAcceptanceFunc(func(text string, lastChar rune) bool {
							return true
						})

					// Create modal for warning message
					modal := tview.NewModal().
						SetText(warningText).
						AddButtons([]string{"Cancel"})

					// Create a flex container to hold both the modal and input field
					modalFlex := tview.NewFlex().
						SetDirection(tview.FlexRow).
						AddItem(modal, 0, 1, true).
						AddItem(inputField, 1, 0, true)

					// Set up input field handler
					inputField.SetDoneFunc(func(key tcell.Key) {
						if key == tcell.KeyEnter {
							if inputField.GetText() == "DELETE" {
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
									// Show success message
									successModal := tview.NewModal().
										SetText(fmt.Sprintf("Successfully deleted job %s", jobName)).
										AddButtons([]string{"OK"}).
										SetDoneFunc(func(buttonIndex int, buttonLabel string) {
											// Refresh the jobs list
											newJobs, err := getJobs()
											if err == nil {
												jobs = newJobs
												updateTable(jobs)
											}
											app.SetRoot(flex, true)
										})
									app.SetRoot(successModal, false)
								}
							} else {
								app.SetRoot(flex, true)
							}
						}
					})

					// Set up modal button handler
					modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						app.SetRoot(flex, true)
					})

					app.SetRoot(modalFlex, true)
					app.SetFocus(inputField)
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
					updateTable(jobs)
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
