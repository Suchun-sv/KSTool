// Package model holds the data types that flow between the k8s layer and the TUI.
package model

import (
	"fmt"
	"sort"
	"time"
)

// Status is a stable enum derived from a Kubernetes Job's state.
type Status string

const (
	StatusPending  Status = "Pending"
	StatusRunning  Status = "Running"
	StatusComplete Status = "Complete"
	StatusFailed   Status = "Failed"
)

// Job is the UI-facing snapshot of a Kubernetes Job.
type Job struct {
	Name        string
	Namespace   string
	Owner       string
	Status      Status
	Succeeded   int32
	Completions int32
	StartTime   *time.Time
	EndTime     *time.Time
	Created     time.Time
	PodCount    int
	GPUCount    int
	GPU         GPU
}

// Completions renders "<succeeded>/<spec>" as the original UI did.
func (j Job) CompletionsLabel() string {
	if j.Completions == 0 {
		return fmt.Sprintf("%d/1", j.Succeeded)
	}
	return fmt.Sprintf("%d/%d", j.Succeeded, j.Completions)
}

// Duration is the wall-clock time the job has been running, or "-" if not started.
func (j Job) Duration() string {
	if j.StartTime == nil {
		return "-"
	}
	end := time.Now()
	if j.EndTime != nil {
		end = *j.EndTime
	}
	return formatDuration(end.Sub(*j.StartTime))
}

// Age returns wall-clock age since creation.
func (j Job) Age() string { return formatDuration(time.Since(j.Created)) }

// PodsLabel mirrors the legacy "N pods" string.
func (j Job) PodsLabel() string { return fmt.Sprintf("%d pods", j.PodCount) }

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd%dh%dm", days, hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh%dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// FilterMode controls which jobs are shown.
type FilterMode int

const (
	FilterAll FilterMode = iota
	FilterRunning
	FilterFailed
	FilterPending
)

func (f FilterMode) Next() FilterMode { return (f + 1) % 4 }

func (f FilterMode) Label() string {
	switch f {
	case FilterAll:
		return "All"
	case FilterRunning:
		return "Running"
	case FilterFailed:
		return "Failed"
	case FilterPending:
		return "Pending"
	}
	return "?"
}

// SortMode controls job ordering.
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

func (s SortMode) Next() SortMode { return (s + 1) % 8 }

func (s SortMode) Label() string {
	switch s {
	case SortAgeDesc:
		return "Age↓"
	case SortAgeAsc:
		return "Age↑"
	case SortGPUCountAsc:
		return "GPU#↑"
	case SortGPUCountDesc:
		return "GPU#↓"
	case SortDurationDesc:
		return "Dur↓"
	case SortDurationAsc:
		return "Dur↑"
	case SortGPUTypeDesc:
		return "GPU Type↓"
	case SortGPUTypeAsc:
		return "GPU Type↑"
	}
	return "?"
}

// Filter applies status + ownership filtering.
func Filter(jobs []Job, mode FilterMode, owner string, onlyOwn bool) []Job {
	out := jobs[:0:0]
	for _, j := range jobs {
		if onlyOwn && j.Owner != owner {
			continue
		}
		switch mode {
		case FilterRunning:
			if j.Status != StatusRunning {
				continue
			}
		case FilterFailed:
			if j.Status != StatusFailed {
				continue
			}
		case FilterPending:
			if j.Status != StatusPending {
				continue
			}
		}
		out = append(out, j)
	}
	return out
}

// Sort orders jobs in-place by the given mode.
func Sort(jobs []Job, mode SortMode) {
	sort.SliceStable(jobs, func(i, k int) bool {
		a, b := jobs[i], jobs[k]
		switch mode {
		case SortAgeDesc:
			return a.Created.After(b.Created)
		case SortAgeAsc:
			return a.Created.Before(b.Created)
		case SortDurationDesc:
			return durationOf(a) > durationOf(b)
		case SortDurationAsc:
			return durationOf(a) < durationOf(b)
		case SortGPUCountAsc:
			return a.GPUCount < b.GPUCount
		case SortGPUCountDesc:
			return a.GPUCount > b.GPUCount
		case SortGPUTypeDesc:
			return a.GPU.Priority() > b.GPU.Priority()
		case SortGPUTypeAsc:
			return a.GPU.Priority() < b.GPU.Priority()
		}
		return false
	})
}

func durationOf(j Job) time.Duration {
	if j.StartTime == nil {
		return 0
	}
	end := time.Now()
	if j.EndTime != nil {
		end = *j.EndTime
	}
	return end.Sub(*j.StartTime)
}

