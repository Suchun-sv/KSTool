package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

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
	VERSION   = "0.2.0"
	AUTHOR    = "suchun"

	EMOJI_WAITING = "⏳"
	EMOJI_WARNING = "⚠️"

	REFRESH_INTERVAL = 2 * time.Second // 添加刷新间隔限制
)

// colours for tview
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

// Job is an internal DTO for UI rendering.
// ------------------------------------------------------------

type Job struct {
	Name        string
	Status      string
	Completions string
	Duration    string
	Age         string
	Pods        string
	GPUInfo     string
}

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

	podList, _ := client.CoreV1().Pods(NAMESPACE).List(ctx, metav1.ListOptions{})
	// group pods by owner Job name
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

		// 从 job 的 spec 中获取 GPU 信息
		gpuInfo := summarizeGPU(pods)

		jobs = append(jobs, Job{
			Name:        j.Name,
			Status:      status,
			Completions: completions(&j),
			Duration:    fmtDuration(j.Status.StartTime, j.Status.CompletionTime),
			Age:         age(j.CreationTimestamp.Time),
			Pods:        fmt.Sprintf("%d pods", len(pods)),
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
	return until.Sub(start.Time).Round(time.Second).String()
}

func age(t time.Time) string {
	return time.Since(t).Round(time.Minute).String()
}

// summarizeGPU inspects the first pod's first container resources & node labels
func summarizeGPU(pods []corev1.Pod) string {
	if len(pods) == 0 {
		return EMOJI_WAITING + " No Pod"
	}

	pod := pods[0]
	c := pod.Spec.Containers[0]
	gpuCount := c.Resources.Limits["nvidia.com/gpu"]
	if gpuCount.IsZero() {
		return "No GPU"
	}

	// 从 node selectors 中获取 GPU 型号
	gpuModel := ""
	for key, value := range pod.Spec.NodeSelector {
		if key == "nvidia.com/gpu.product" {
			gpuModel = value
			break
		}
	}

	// 提取简化的 GPU 型号和显存信息
	var modelType string
	var memory string

	// 提取 GPU 类型
	if strings.Contains(gpuModel, "A100") {
		modelType = "A100"
	} else if strings.Contains(gpuModel, "H100") {
		modelType = "H100"
	} else if strings.Contains(gpuModel, "H200") {
		modelType = "H200"
	}

	// 提取显存大小
	if strings.Contains(gpuModel, "40GB") || strings.Contains(gpuModel, "40G") {
		memory = "40G"
	} else if strings.Contains(gpuModel, "80GB") || strings.Contains(gpuModel, "80G") {
		memory = "80G"
	}

	// 格式化输出
	if modelType == "" {
		return fmt.Sprintf("%s GPU", gpuCount.String())
	}
	if memory == "" {
		return fmt.Sprintf("%s %s", gpuCount.String(), modelType)
	}
	return fmt.Sprintf("%s %s-%s", gpuCount.String(), modelType, memory)
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
(d)elete (r)efresh (ctrl+c)exit
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

	headers := []string{"NAME", "STATUS", "COMPLETIONS", "DURATION", "AGE", "PODS", "GPU INFO"}
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
		table.SetCell(i+1, 6, tview.NewTableCell(j.GPUInfo).SetTextColor(getGPUColor(j.GPUInfo)))
	}
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

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(createASCIIArt(), 7, 0, false)
	table := createTable()
	flex.AddItem(table, 0, 1, true)
	flex.AddItem(createVersionInfo(), 1, 0, false)

	updateTable(table, jobs)

	table.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyEscape:
			app.Stop()
		case tcell.KeyRune:
			switch ev.Rune() {
			case 'r':
				// 检查是否达到刷新间隔
				if time.Since(lastRefresh) < REFRESH_INTERVAL {
					return ev
				}
				// 刷新
				if newJobs, err := getJobs(ctx); err == nil {
					updateTable(table, newJobs)
					lastRefresh = time.Now()
				}
			case 'd':
				row, _ := table.GetSelection()
				if row == 0 { // header
					return ev
				}
				jobName := table.GetCell(row, 0).Text
				jobStatus := table.GetCell(row, 1).Text

				modal := createDeleteModal(app, flex, ctx, jobName, jobStatus, table)
				app.SetRoot(modal, true)
				app.SetFocus(modal)
			}
		}
		return ev
	})

	if err := app.SetRoot(flex, true).SetFocus(table).Run(); err != nil {
		panic(err)
	}
}

func createDeleteModal(app *tview.Application, root *tview.Flex, ctx context.Context, jobName, jobStatus string, table *tview.Table) *tview.Flex {
	modalFlex := tview.NewFlex().SetDirection(tview.FlexRow)

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
