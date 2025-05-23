package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v3"

	"github.com/suchun/kstool/src"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ------------------------------------------------------------
// Constants & Types
// ------------------------------------------------------------
const (
	NAMESPACE = "eidf029ns"
	APP_NAME  = "KSTool"
	VERSION   = "1.1.3"
	AUTHOR    = "Beining Yang@LFCS"
	USER_LABEL = "eidf/user"

	EMOJI_WAITING = "⏳"
	EMOJI_WARNING = "⚠️"

	REFRESH_INTERVAL = 2 * time.Second // Add refresh interval limit
)

// Colors for tview
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

// Colors corresponding to GPU count
var gpuColors = []tcell.Color{
	tcell.ColorWhite,  // 0
	tcell.ColorYellow, // 1
	tcell.ColorOrange, // 2
	tcell.ColorRed,    // 3
	tcell.ColorRed,    // 4
	tcell.ColorRed,    // 5
	tcell.ColorRed,    // 6
	tcell.ColorRed,    // 7
	tcell.ColorRed,    // 8
}

// Job is an internal DTO for UI rendering.
// ------------------------------------------------------------

type Job struct {
	Name        string
	Status      string
	Completions string
	Duration    string
	Age         string
	Pods        string
	GPUCount    int
	GPUInfo     string
}

// Add status filter mode
type FilterMode int

const (
	FilterAll FilterMode = iota
	FilterRunning
	FilterFailed
	FilterPending
)

// Add user filter mode
type UserFilterMode int

const (
	UserFilterAll UserFilterMode = iota
	UserFilterCurrent
)

// Add sort mode
type SortMode int

const (
	SortAgeDesc SortMode = iota
	SortAgeAsc
	SortGPUCountAsc
	SortGPUCountDesc
	SortDurationDesc
	SortDurationAsc
	SortGPUTypeDesc
	SortGPUTypeAsc
)

// ------------------------------------------------------------
// Kubernetes client helpers
// ------------------------------------------------------------

var client *kubernetes.Clientset

func newClient() (*kubernetes.Clientset, error) {
	// Try in-cluster config first
	cfg, err := rest.InClusterConfig()
	if err == nil {
		cfg.Timeout = 5 * time.Second
		return kubernetes.NewForConfig(cfg)
	}

	// Not in a cluster: try KUBECONFIG env var or default location
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot get user home directory: %w", err)
		}
		kubeconfig = homeDir + "/.kube/config"
	}

	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig from %s: %w", kubeconfig, err)
	}

	cfg.Timeout = 5 * time.Second
	return kubernetes.NewForConfig(cfg)
}

func init() {
	var err error
	client, err = newClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create k8s client: %v\n", err)
		os.Exit(1)
	}
}

// ------------------------------------------------------------
// Business logic (replaces kubectl+grep)
// ------------------------------------------------------------

func getJobs(ctx context.Context) ([]Job, error) {
	jobList, err := client.BatchV1().Jobs(NAMESPACE).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Get all pods at once
	podList, err := client.CoreV1().Pods(NAMESPACE).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Group pods by job name
	jobPods := make(map[string][]corev1.Pod)
	for _, p := range podList.Items {
		if owner := metav1.GetControllerOf(&p); owner != nil && owner.Kind == "Job" {
			jobPods[owner.Name] = append(jobPods[owner.Name], p)
		}
	}

	jobs := make([]Job, 0, len(jobList.Items))
	for _, j := range jobList.Items {
		pods := jobPods[j.Name]
		status := deriveStatus(j)

		// Calculate GPU count
		gpuCount := 0
		if len(j.Spec.Template.Spec.Containers) > 0 {
			gpuLimit := j.Spec.Template.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"]
			if !gpuLimit.IsZero() {
				gpuCount = int(gpuLimit.Value())
			}
		}

		// Get GPU information from job spec
		gpuInfo := summarizeGPU(&j)

		jobs = append(jobs, Job{
			Name:        j.Name,
			Status:      status,
			Completions: completions(&j),
			Duration:    fmtDuration(j.Status.StartTime, j.Status.CompletionTime),
			Age:         age(j.CreationTimestamp.Time),
			Pods:        fmt.Sprintf("%d pods", len(pods)),
			GPUCount:    gpuCount,
			GPUInfo:     gpuInfo,
		})
	}
	return jobs, nil
}

func deriveStatus(j batchv1.Job) string {
	switch {
	case j.Status.Active > 0:
		return "Running"
	case j.Status.Succeeded > 0:
		return "Complete"
	case j.Status.Failed > 0:
		return "Failed"
	default:
		return "Pending"
	}
}

func completions(j *batchv1.Job) string {
	if j.Spec.Completions == nil {
		return fmt.Sprintf("%d/1", j.Status.Succeeded)
	}
	return fmt.Sprintf("%d/%d", j.Status.Succeeded, *j.Spec.Completions)
}

func fmtDuration(start, end *metav1.Time) string {
	if start == nil {
		return "‑"
	}
	until := time.Now()
	if end != nil {
		until = end.Time
	}
	duration := until.Sub(start.Time)

	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}

func age(t time.Time) string {
	duration := time.Since(t)

	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}

// summarizeGPU inspects the job's spec to get GPU information
func summarizeGPU(job *batchv1.Job) string {
	if job == nil || len(job.Spec.Template.Spec.Containers) == 0 {
		return EMOJI_WAITING + " No Container"
	}

	// Get GPU model from node selectors
	gpuModel := ""
	for key, value := range job.Spec.Template.Spec.NodeSelector {
		if key == "nvidia.com/gpu.product" {
			gpuModel = value
			break
		}
	}

	// Extract simplified GPU model and memory information
	var modelType string
	var memory string

	// Extract GPU type
	if strings.Contains(gpuModel, "A100") {
		modelType = "A100"
	} else if strings.Contains(gpuModel, "H100") {
		modelType = "H100"
	} else if strings.Contains(gpuModel, "H200") {
		modelType = "H200"
	}

	// Extract memory size
	if strings.Contains(gpuModel, "40GB") || strings.Contains(gpuModel, "40G") {
		memory = "40G"
	} else if strings.Contains(gpuModel, "80GB") || strings.Contains(gpuModel, "80G") {
		memory = "80G"
	}

	// Format output
	if modelType == "" {
		return "Unknown"
	}

	// Check if job is running
	isRunning := job.Status.Active > 0
	if !isRunning {
		if memory == "" {
			return EMOJI_WAITING + " " + modelType
		}
		return EMOJI_WAITING + " " + modelType + "-" + memory
	}

	if memory == "" {
		return modelType
	}
	return fmt.Sprintf("%s-%s", modelType, memory)
}

// ------------------------------------------------------------
// Delete job via API
// ------------------------------------------------------------

func deleteJob(ctx context.Context, jobName string) error {
	prog := metav1.DeletePropagationForeground
	opts := metav1.DeleteOptions{
		PropagationPolicy: &prog,
	}
	return client.BatchV1().Jobs(NAMESPACE).Delete(ctx, jobName, opts)
}

// ------------------------------------------------------------
// tview UI helpers (mostly unchanged)
// ------------------------------------------------------------

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

func getGPUColor(info string) tcell.Color {
	switch {
	case strings.HasPrefix(info, EMOJI_WAITING):
		return COLOR_WAITING
	case strings.Contains(info, "H200"):
		return COLOR_H200
	case strings.Contains(info, "H100"):
		return COLOR_H100
	case strings.Contains(info, "A100"):
		return COLOR_A100
	case strings.Contains(info, "No GPU"):
		return COLOR_NO_GPU
	default:
		return COLOR_DEFAULT
	}
}

func createASCIIArt() *tview.TextView {
	art := `
 ██╗  ██╗███████╗████████╗ ██████╗  ██████╗ ██╗     
 ██║ ██╔╝██╔════╝╚══██╔══╝██╔═══██╗██╔═══██╗██║     
 ██████╔╝███████╗   ██║   ██║   ██║██║   ██║██║     
 ██╔═██╗ ╚════██║   ██║   ██║   ██║██║   ██║██║     
 ██║  ██╗███████║   ██║   ╚██████╔╝╚██████╔╝███████╗
 ╚═╝  ╚═╝╚══════╝   ╚═╝    ╚═════╝  ╚═════╝ ╚══════╝
===================================================
(d)elete (r)efresh (e)nter (n)ew config (ctrl+c)exit
`
	return tview.NewTextView().
		SetTextAlign(tview.AlignLeft).
		SetText(art).
		SetTextColor(COLOR_A100)
}

func createVersionInfo() *tview.TextView {
	return tview.NewTextView().
		SetTextAlign(tview.AlignLeft).
		SetText(fmt.Sprintf("%s@%s by %s", APP_NAME, VERSION, AUTHOR)).
		SetTextColor(COLOR_DEFAULT)
}

func createTable() *tview.Table {
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ')

	headers := []string{"NAME", "STATUS", "COMPLETIONS", "DURATION", "AGE", "PODS", "GPU", "GPU INFO"}
	for i, h := range headers {
		table.SetCell(0, i, tview.NewTableCell(h).
			SetTextColor(COLOR_HEADER).
			SetAlign(tview.AlignLeft).
			SetSelectable(false))
	}

	table.SetDrawFunc(func(s tcell.Screen, x, y, w, h int) (int, int, int, int) {
		sty := tcell.StyleDefault.Foreground(tcell.ColorWhite)
		for i := x; i < x+w; i++ {
			s.SetContent(i, y, tcell.RuneHLine, nil, sty)
			s.SetContent(i, y+h-1, tcell.RuneHLine, nil, sty)
		}
		return x, y, w, h
	})

	return table
}

func updateTable(table *tview.Table, jobs []Job) {
	for i := table.GetRowCount() - 1; i > 0; i-- {
		table.RemoveRow(i)
	}
	for i, j := range jobs {
		table.SetCell(i+1, 0, tview.NewTableCell(j.Name))
		table.SetCell(i+1, 1, tview.NewTableCell(j.Status).SetTextColor(getStatusColor(j.Status)))
		table.SetCell(i+1, 2, tview.NewTableCell(j.Completions))
		table.SetCell(i+1, 3, tview.NewTableCell(j.Duration))
		table.SetCell(i+1, 4, tview.NewTableCell(j.Age))
		table.SetCell(i+1, 5, tview.NewTableCell(j.Pods))

		// 使用 Job 结构体中的 GPUCount
		table.SetCell(i+1, 6, tview.NewTableCell(fmt.Sprintf("%d", j.GPUCount)).
			SetTextColor(getGPUCountColor(j.GPUCount)))

		table.SetCell(i+1, 7, tview.NewTableCell(j.GPUInfo).SetTextColor(getGPUColor(j.GPUInfo)))
	}
}

// 根据 GPU 数量获取对应的颜色
func getGPUCountColor(count int) tcell.Color {
	if count >= len(gpuColors) {
		return gpuColors[len(gpuColors)-1]
	}
	return gpuColors[count]
}

// ------------------------------------------------------------
// main
// ------------------------------------------------------------

func main() {
	ctx := context.Background()
	jobs, err := getJobs(ctx)
	if err != nil {
		panic(err)
	}

	app := tview.NewApplication()
	lastRefresh := time.Now()
	currentFilter := FilterAll
	currentSort := SortAgeDesc

	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// ASCII art
	flex.AddItem(createASCIIArt(), 7, 0, false)

	// Filter status display
	filterText := tview.NewTextView().
		SetTextAlign(tview.AlignLeft).
		SetText("(F)ilter: All | (H)ide Others | (S)ort: Age↓ | (R)efresh | (D)elete | (E)nter | (C)onfig | (N)ew Config").
		SetTextColor(COLOR_DEFAULT)
	flex.AddItem(filterText, 1, 0, false)

	// Table
	table := createTable()
	flex.AddItem(table, 0, 1, true)

	// Version info
	flex.AddItem(createVersionInfo(), 1, 0, false)

	// CommandHandler
	commandHandler := NewCommandHandler(app, flex, table, ctx, jobs, lastRefresh, currentFilter, currentSort, filterText)

	// Update table function
	updateTableWithFilter := func() {
		commandHandler.updateTableWithFilter()
	}

	updateTableWithFilter()

	table.SetInputCapture(commandHandler.HandleCommand)

	if err := app.SetRoot(flex, true).SetFocus(table).Run(); err != nil {
		panic(err)
	}
}

// Get text description for sort mode
func getSortText(mode SortMode) string {
	switch mode {
	case SortAgeDesc:
		return "Age↓"
	case SortAgeAsc:
		return "Age↑"
	case SortGPUCountAsc:
		return "GPU Number↑"
	case SortGPUCountDesc:
		return "GPU Number↓"
	case SortDurationDesc:
		return "Duration↓"
	case SortDurationAsc:
		return "Duration↑"
	case SortGPUTypeDesc:
		return "GPU Type↓"
	case SortGPUTypeAsc:
		return "GPU Type↑"
	default:
		return "Unknown"
	}
}

// Get priority for GPU type
func getGPUTypePriority(gpuType string) int {
	// Base priority
	basePriority := 0
	// Memory priority
	memoryPriority := 0

	// Determine base priority
	switch {
	case strings.Contains(gpuType, "H200"):
		basePriority = 300
	case strings.Contains(gpuType, "H100"):
		basePriority = 200
	case strings.Contains(gpuType, "A100"):
		basePriority = 100
	default:
		basePriority = 0
	}

	// Determine memory priority
	switch {
	case strings.Contains(gpuType, "80G") || strings.Contains(gpuType, "80GB"):
		memoryPriority = 2
	case strings.Contains(gpuType, "40G") || strings.Contains(gpuType, "40GB"):
		memoryPriority = 1
	default:
		memoryPriority = 0
	}

	return basePriority + memoryPriority
}

// Parse duration string to minutes
func parseDuration(duration string) int64 {
	if duration == "‑" {
		return 0
	}

	// Parse time format, e.g., "1d2h3m" or "2h3m" or "3m"
	var days, hours, minutes int64
	// Try to parse full format first
	if strings.Contains(duration, "d") {
		fmt.Sscanf(duration, "%dd%dh%dm", &days, &hours, &minutes)
	} else if strings.Contains(duration, "h") {
		fmt.Sscanf(duration, "%dh%dm", &hours, &minutes)
	} else {
		fmt.Sscanf(duration, "%dm", &minutes)
	}
	return days*24*60 + hours*60 + minutes
}

// Parse age string to minutes
func parseAge(age string) int64 {
	// Parse time format, e.g., "1d2h3m" or "2h3m" or "3m"
	var days, hours, minutes int64
	// Try to parse full format first
	if strings.Contains(age, "d") {
		fmt.Sscanf(age, "%dd%dh%dm", &days, &hours, &minutes)
	} else if strings.Contains(age, "h") {
		fmt.Sscanf(age, "%dh%dm", &hours, &minutes)
	} else {
		fmt.Sscanf(age, "%dm", &minutes)
	}
	return days*24*60 + hours*60 + minutes
}

// Sort function
func sortJobs(jobs []Job, mode SortMode) {
	switch mode {
	case SortAgeDesc:
		sort.Slice(jobs, func(i, j int) bool {
			ageI := parseAge(jobs[i].Age)
			ageJ := parseAge(jobs[j].Age)
			return ageI > ageJ
		})
	case SortAgeAsc:
		sort.Slice(jobs, func(i, j int) bool {
			ageI := parseAge(jobs[i].Age)
			ageJ := parseAge(jobs[j].Age)
			return ageI < ageJ
		})
	case SortDurationDesc:
		sort.Slice(jobs, func(i, j int) bool {
			durationI := parseDuration(jobs[i].Duration)
			durationJ := parseDuration(jobs[j].Duration)
			return durationI > durationJ
		})
	case SortDurationAsc:
		sort.Slice(jobs, func(i, j int) bool {
			durationI := parseDuration(jobs[i].Duration)
			durationJ := parseDuration(jobs[j].Duration)
			return durationI < durationJ
		})
	case SortGPUCountAsc:
		sort.Slice(jobs, func(i, j int) bool {
			return jobs[i].GPUCount < jobs[j].GPUCount
		})
	case SortGPUCountDesc:
		sort.Slice(jobs, func(i, j int) bool {
			return jobs[i].GPUCount > jobs[j].GPUCount
		})
	case SortGPUTypeDesc:
		sort.Slice(jobs, func(i, j int) bool {
			return getGPUTypePriority(jobs[i].GPUInfo) > getGPUTypePriority(jobs[j].GPUInfo)
		})
	case SortGPUTypeAsc:
		sort.Slice(jobs, func(i, j int) bool {
			return getGPUTypePriority(jobs[i].GPUInfo) < getGPUTypePriority(jobs[j].GPUInfo)
		})
	}
}

// Add filter function
func filterJobsByStatus(jobs []Job, status string) []Job {
	var filtered []Job
	for _, job := range jobs {
		if job.Status == status {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

func createDeleteModal(app *tview.Application, root *tview.Flex, ctx context.Context, jobName, jobStatus string, table *tview.Table) *tview.Flex {
	modalFlex := tview.NewFlex().SetDirection(tview.FlexRow)

	currentUser := os.Getenv("USER")
	if !strings.HasPrefix(jobName, currentUser) {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Cannot delete job '%s': You can only delete your own jobs (prefix: %s)", jobName, currentUser)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				app.SetRoot(root, true)
			})
		modalFlex.AddItem(modal, 0, 1, true)
		return modalFlex
	}

	warningText := fmt.Sprintf("%s WARNING! Delete job '%s' (status: %s)?", EMOJI_WARNING, jobName, jobStatus)

	modal := tview.NewModal().
		SetText(warningText).
		AddButtons([]string{"Cancel", "Confirm"})

	modalFlex.AddItem(modal, 0, 1, true)

	modal.SetDoneFunc(func(idx int, label string) {
		if label == "Confirm" {
			if err := deleteJob(ctx, jobName); err != nil {
				errModal := tview.NewModal().
					SetText(fmt.Sprintf("Error deleting job '%s':\n%v\n\nPress OK to continue", jobName, err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						app.SetRoot(root, true)
					})
				app.SetRoot(errModal, true)
			} else {
				// 从表格中移除被删除的 job
				for i := 1; i < table.GetRowCount(); i++ {
					if table.GetCell(i, 0).Text == jobName {
						table.RemoveRow(i)
						break
					}
				}
				successModal := tview.NewModal().
					SetText(fmt.Sprintf("Job '%s' deleted successfully.\nPress OK to continue", jobName)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						app.SetRoot(root, true)
					})
				app.SetRoot(successModal, true)
			}
		} else {
			app.SetRoot(root, true)
		}
	})

	return modalFlex
}

func execPod(ctx context.Context, jobName string) error {
	// Get pods for the job
	pods, err := client.CoreV1().Pods(NAMESPACE).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found for job %s", jobName)
	}

	// Get the first running pod
	var targetPod *corev1.Pod
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			targetPod = &pod
			break
		}
	}

	if targetPod == nil {
		return fmt.Errorf("no running pods found for job %s", jobName)
	}

	// Execute kubectl exec command
	cmd := exec.Command("kubectl", "exec", "-it", "-n", NAMESPACE, targetPod.Name, "--", "bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command and get the error
	err = cmd.Run()

	// Clear the screen after returning from exec
	fmt.Print("\033[H\033[2J")

	return err
}

// Get job YAML configuration
func getJobYAML(ctx context.Context, jobName string) (string, error) {
	job, err := client.BatchV1().Jobs(NAMESPACE).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get job: %w", err)
	}

	// Convert job to YAML
	jobYAML, err := yaml.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job to YAML: %w", err)
	}

	return string(jobYAML), nil
}

// CommandHandler handles all command operations
type CommandHandler struct {
	app            *tview.Application
	flex           *tview.Flex
	table         *tview.Table
	ctx           context.Context
	jobs          []Job
	lastRefresh   time.Time
	currentFilter FilterMode
	currentSort   SortMode
	currentUser   string
	filterText    *tview.TextView
	showOnlyUser  bool
}

// NewCommandHandler creates a new CommandHandler
func NewCommandHandler(app *tview.Application, flex *tview.Flex, table *tview.Table, ctx context.Context, jobs []Job, lastRefresh time.Time, currentFilter FilterMode, currentSort SortMode, filterText *tview.TextView) *CommandHandler {
	return &CommandHandler{
		app:            app,
		flex:           flex,
		table:         table,
		ctx:           ctx,
		jobs:          jobs,
		lastRefresh:   lastRefresh,
		currentFilter: currentFilter,
		currentSort:   currentSort,
		currentUser:   os.Getenv("USER"),
		filterText:    filterText,
		showOnlyUser:  false,
	}
}

// HandleCommand handles all commands
func (h *CommandHandler) HandleCommand(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyEscape:
		h.app.Stop()
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'r':
			return h.handleRefresh()
		case 'f':
			return h.handleFilter()
		case 'h':
			h.showOnlyUser = !h.showOnlyUser
			h.updateTableWithFilter()
			return nil
		case 's':
			return h.handleSort()
		case 'd':
			return h.handleDelete()
		case 'e':
			return h.handleEnter()
		case 'c':
			return h.handleConfig()
		case 'n':
			return h.handleNewConfig()
		}
	}
	return ev
}

// handleRefresh handles the refresh command
func (h *CommandHandler) handleRefresh() *tcell.EventKey {
	if time.Since(h.lastRefresh) < REFRESH_INTERVAL {
		return nil
	}
	if newJobs, err := getJobs(h.ctx); err == nil {
		h.jobs = newJobs
		h.updateTableWithFilter()
		h.lastRefresh = time.Now()
	}
	return nil
}

// handleFilter handles the filter command
func (h *CommandHandler) handleFilter() *tcell.EventKey {
	h.currentFilter = (h.currentFilter + 1) % 4
	h.updateTableWithFilter()
	return nil
}

// handleSort handles the sort command
func (h *CommandHandler) handleSort() *tcell.EventKey {
	h.currentSort = (h.currentSort + 1) % 8
	h.updateTableWithFilter()
	return nil
}

// handleDelete handles the delete command
func (h *CommandHandler) handleDelete() *tcell.EventKey {
	row, _ := h.table.GetSelection()
	if row == 0 { // header
		return nil
	}
	jobName := h.table.GetCell(row, 0).Text
	jobStatus := h.table.GetCell(row, 1).Text

	// Retrieve job to get labels
	job, err := client.BatchV1().Jobs(NAMESPACE).Get(h.ctx, jobName, metav1.GetOptions{})
	if err != nil {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Error retrieving job '%s':\n%v\n\nPress OK to continue", jobName, err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				h.app.SetRoot(h.flex, true)
			})
		h.app.SetRoot(modal, true)
		return nil
	}

	// Check if the job belongs to the current user
	owner, exists := job.Labels[USER_LABEL]
	if !exists || owner != h.currentUser {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Cannot delete job '%s': You can only delete your own jobs (owner: %s)", jobName, owner)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				h.app.SetRoot(h.flex, true)
			})
		h.app.SetRoot(modal, true)
		return nil
	}

	// Format labels for display
	labels := []string{}
	for key, value := range job.Labels {
		labels = append(labels, fmt.Sprintf("%s: %s", key, value))
	}
	labelText := strings.Join(labels, "\n")

	warningText := fmt.Sprintf("%s WARNING! Delete job '%s' (status: %s)?\nLabels:\n%s", EMOJI_WARNING, jobName, jobStatus, labelText)

	modal := tview.NewModal().
		SetText(warningText).
		AddButtons([]string{"Cancel", "Confirm"})

	modal.SetDoneFunc(func(idx int, label string) {
		if label == "Confirm" {
			if err := deleteJob(h.ctx, jobName); err != nil {
				errModal := tview.NewModal().
					SetText(fmt.Sprintf("Error deleting job '%s':\n%v\n\nPress OK to continue", jobName, err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						h.app.SetRoot(h.flex, true)
					})
				h.app.SetRoot(errModal, true)
			} else {
				// Log the deletion
				user, _ := src.GetCurrentUser()
				timestamp := time.Now().Format(time.RFC3339)
				logMessage := fmt.Sprintf("Timestamp: %s, User: %s, Deleted Job: %s", timestamp, user, jobName)
				src.LogToSyslog(logMessage)

				// Remove the deleted job from the table
				for i := 1; i < h.table.GetRowCount(); i++ {
					if h.table.GetCell(i, 0).Text == jobName {
						h.table.RemoveRow(i)
						break
					}
				}
				successModal := tview.NewModal().
					SetText(fmt.Sprintf("Job '%s' deleted successfully.\nPress OK to continue", jobName)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						h.app.SetRoot(h.flex, true)
					})
				h.app.SetRoot(successModal, true)
			}
		} else {
			h.app.SetRoot(h.flex, true)
		}
	})

	h.app.SetRoot(modal, true)
	h.app.SetFocus(modal)
	return nil
}

// handleEnter handles the enter command
func (h *CommandHandler) handleEnter() *tcell.EventKey {
	row, _ := h.table.GetSelection()
	if row == 0 { // header
		return nil
	}
	jobName := h.table.GetCell(row, 0).Text
	jobStatus := h.table.GetCell(row, 1).Text

	// Retrieve job to get labels
	job, err := client.BatchV1().Jobs(NAMESPACE).Get(h.ctx, jobName, metav1.GetOptions{})
	if err != nil {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Error retrieving job '%s':\n%v\n\nPress OK to continue", jobName, err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				h.app.SetRoot(h.flex, true)
			})
		h.app.SetRoot(modal, true)
		return nil
	}

	// Check if the job belongs to the current user
	owner, exists := job.Labels[USER_LABEL]
	if !exists || owner != h.currentUser {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Cannot exec into job '%s': You can only exec into your own jobs (owner: %s)", jobName, owner)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				h.app.SetRoot(h.flex, true)
			})
		h.app.SetRoot(modal, true)
		return nil
	}

	if jobStatus != "Running" {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Cannot exec into job '%s': job is not running (status: %s)", jobName, jobStatus)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				h.app.SetRoot(h.flex, true)
			})
		h.app.SetRoot(modal, true)
		return nil
	}

	// Log the enter action
	user, _ := src.GetCurrentUser()
	timestamp := time.Now().Format(time.RFC3339)
	logMessage := fmt.Sprintf("Timestamp: %s, User: %s, Entered Job: %s", timestamp, user, jobName)
	src.LogToSyslog(logMessage)

	// Stop the TUI before executing kubectl
	h.app.Stop()

	// Execute kubectl exec
	if err := execPod(h.ctx, jobName); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to exec into pod: %v\n", err)
	}

	// Restart the TUI with a fresh context
	h.app = tview.NewApplication()
	h.app.SetRoot(h.flex, true).SetFocus(h.table)
	if err := h.app.Run(); err != nil {
		panic(err)
	}
	return nil
}

// handleConfig handles the config command
func (h *CommandHandler) handleConfig() *tcell.EventKey {
	row, _ := h.table.GetSelection()
	if row == 0 { // header
		return nil
	}
	jobName := h.table.GetCell(row, 0).Text

	// Get job YAML
	yamlContent, err := getJobYAML(h.ctx, jobName)
	if err != nil {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Error getting job config for '%s':\n%v\n\nPress OK to continue", jobName, err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				h.app.SetRoot(h.flex, true)
			})
		h.app.SetRoot(modal, true)
		return nil
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("job-%s-*.yaml", jobName))
	if err != nil {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Error creating temporary file:\n%v\n\nPress OK to continue", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				h.app.SetRoot(h.flex, true)
			})
		h.app.SetRoot(modal, true)
		return nil
	}
	defer os.Remove(tmpFile.Name())

	// Write YAML to temporary file
	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Error writing to temporary file:\n%v\n\nPress OK to continue", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				h.app.SetRoot(h.flex, true)
			})
		h.app.SetRoot(modal, true)
		return nil
	}
	tmpFile.Close()

	// Stop the TUI before executing vim
	h.app.Stop()

	// Execute vim in read-only mode
	cmd := exec.Command("vim", "-R", tmpFile.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open vim: %v\n", err)
	}

	// Restart the TUI with a fresh context
	h.app = tview.NewApplication()
	h.app.SetRoot(h.flex, true).SetFocus(h.table)
	if err := h.app.Run(); err != nil {
		panic(err)
	}
	return nil
}

// handleNewConfig handles the new config command
func (h *CommandHandler) handleNewConfig() *tcell.EventKey {
	// Create new job form
	createForm := src.NewCreateJobForm(h.app, h.ctx, func() {
		// Refresh data after closing the form
		if newJobs, err := getJobs(h.ctx); err == nil {
			h.jobs = newJobs
			h.updateTableWithFilter()
		} else {
			log.Printf("Error getting jobs: %v", err)
		}
		h.app.SetRoot(h.flex, true)
		h.app.SetFocus(h.table)
	})

	if createForm == nil {
		log.Println("Failed to create job form")
		return nil
	}

	// Only call Show if createForm is not nil
	createForm.Show()
	return nil
}

// updateTableWithFilter updates the table with current filter settings
func (h *CommandHandler) updateTableWithFilter() {
	var filteredJobs []Job
	// Apply user filter first
	if h.showOnlyUser {
		for _, job := range h.jobs {
			if strings.HasPrefix(job.Name, h.currentUser) {
				filteredJobs = append(filteredJobs, job)
			}
		}
	} else {
		filteredJobs = h.jobs
	}

	// Then apply status filter
	switch h.currentFilter {
	case FilterAll:
		h.filterText.SetText(fmt.Sprintf("(F)ilter: All | (H)ide Others: %v | (S)ort: %s | (R)efresh | (D)elete | (E)nter | (C)onfig | (N)ew Config",
			h.showOnlyUser, getSortText(h.currentSort)))
	case FilterRunning:
		filteredJobs = filterJobsByStatus(filteredJobs, "Running")
		h.filterText.SetText(fmt.Sprintf("(F)ilter: Running | (H)ide Others: %v | (S)ort: %s | (R)efresh | (D)elete | (E)nter | (C)onfig | (N)ew Config",
			h.showOnlyUser, getSortText(h.currentSort)))
	case FilterFailed:
		filteredJobs = filterJobsByStatus(filteredJobs, "Failed")
		h.filterText.SetText(fmt.Sprintf("(F)ilter: Failed | (H)ide Others: %v | (S)ort: %s | (R)efresh | (D)elete | (E)nter | (C)onfig | (N)ew Config",
			h.showOnlyUser, getSortText(h.currentSort)))
	case FilterPending:
		filteredJobs = filterJobsByStatus(filteredJobs, "Pending")
		h.filterText.SetText(fmt.Sprintf("(F)ilter: Pending | (H)ide Others: %v | (S)ort: %s | (R)efresh | (D)elete | (E)nter | (C)onfig | (N)ew Config",
			h.showOnlyUser, getSortText(h.currentSort)))
	}

	// Apply sorting
	sortJobs(filteredJobs, h.currentSort)
	updateTable(h.table, filteredJobs)
}
