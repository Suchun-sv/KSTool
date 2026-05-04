package k8s

import (
	"bytes"
	"io"

	"github.com/suchun/kstool/internal/model"

	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/yaml"
)

const gpuLimitKey = "nvidia.com/gpu"
const gpuProductSelector = "nvidia.com/gpu.product"

func jobFromAPI(j *batchv1.Job, podCount int, userLabel, _ string) model.Job {
	status := model.StatusPending
	switch {
	case j.Status.Active > 0:
		status = model.StatusRunning
	case j.Status.Succeeded > 0:
		status = model.StatusComplete
	case j.Status.Failed > 0:
		status = model.StatusFailed
	}

	gpuCount := 0
	gpuModel := ""
	if len(j.Spec.Template.Spec.Containers) > 0 {
		if q, ok := j.Spec.Template.Spec.Containers[0].Resources.Limits[gpuLimitKey]; ok {
			gpuCount = int(q.Value())
		}
	}
	if v, ok := j.Spec.Template.Spec.NodeSelector[gpuProductSelector]; ok {
		gpuModel = v
	}
	gpu := model.ParseGPU(gpuModel)
	gpu.Idle = status != model.StatusRunning && gpu.Model != ""

	completions := int32(0)
	if j.Spec.Completions != nil {
		completions = *j.Spec.Completions
	}

	out := model.Job{
		Name:        j.Name,
		Namespace:   j.Namespace,
		Owner:       j.Labels[userLabel],
		Status:      status,
		Succeeded:   j.Status.Succeeded,
		Completions: completions,
		Created:     j.CreationTimestamp.Time,
		PodCount:    podCount,
		GPUCount:    gpuCount,
		GPU:         gpu,
	}
	if j.Status.StartTime != nil {
		t := j.Status.StartTime.Time
		out.StartTime = &t
	}
	if j.Status.CompletionTime != nil {
		t := j.Status.CompletionTime.Time
		out.EndTime = &t
	}
	return out
}

func marshalYAML(j *batchv1.Job) ([]byte, error) {
	return yaml.Marshal(j)
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
