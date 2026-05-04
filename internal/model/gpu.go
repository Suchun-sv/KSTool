package model

import "strings"

// GPU describes the GPU model attached to a Job.
type GPU struct {
	Model  string // "H200" / "H100" / "A100" / ""
	Memory string // "40G" / "80G" / ""
	Idle   bool   // true when the job is waiting (not running)
}

// ParseGPU extracts a GPU model+memory from a node-selector value such as
// "NVIDIA-H100-80GB-HBM3". An empty input yields a zero-value GPU.
func ParseGPU(nodeSelector string) GPU {
	if nodeSelector == "" {
		return GPU{}
	}
	g := GPU{}
	switch {
	case strings.Contains(nodeSelector, "H200"):
		g.Model = "H200"
	case strings.Contains(nodeSelector, "H100"):
		g.Model = "H100"
	case strings.Contains(nodeSelector, "A100"):
		g.Model = "A100"
	}
	switch {
	case strings.Contains(nodeSelector, "80G"):
		g.Memory = "80G"
	case strings.Contains(nodeSelector, "40G"):
		g.Memory = "40G"
	}
	return g
}

// Label returns the user-facing label, prefixed with an hourglass when idle.
func (g GPU) Label() string {
	if g.Model == "" {
		return "Unknown"
	}
	core := g.Model
	if g.Memory != "" {
		core += "-" + g.Memory
	}
	if g.Idle {
		return "⏳ " + core
	}
	return core
}

// Priority gives sortable weight to GPU types.
func (g GPU) Priority() int {
	base := 0
	switch g.Model {
	case "H200":
		base = 300
	case "H100":
		base = 200
	case "A100":
		base = 100
	}
	mem := 0
	switch g.Memory {
	case "80G":
		mem = 2
	case "40G":
		mem = 1
	}
	return base + mem
}
