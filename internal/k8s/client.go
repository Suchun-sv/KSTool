// Package k8s wraps client-go behind a small interface that the TUI consumes.
// The wrapper deliberately avoids shelling out to kubectl or envsubst.
package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/suchun/kstool/internal/model"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// Client is the abstraction the TUI talks to. A fake implementation lives in
// fake.go for tests.
type Client interface {
	ListJobs(ctx context.Context) ([]model.Job, error)
	GetJob(ctx context.Context, name string) (*batchv1.Job, error)
	GetJobYAML(ctx context.Context, name string) ([]byte, error)
	DeleteJob(ctx context.Context, name string) error
	CreateJob(ctx context.Context, manifest []byte, dryRun bool) (*batchv1.Job, error)
	ExecJob(ctx context.Context, name string, stdin io.Reader, stdout, stderr io.Writer, tty bool) error
	JobLogs(ctx context.Context, name string, tailLines int64) ([]byte, error)
	StreamJobLogs(ctx context.Context, name string, tailLines int64) (io.ReadCloser, error)
	DescribeJob(ctx context.Context, name string) (*JobSnapshot, error)
	Namespace() string
	UserLabel() string
}

// JobSnapshot is the data backing the describe view: the live Job, its pods,
// and the merged event timeline (job events + pod events) sorted oldest first.
type JobSnapshot struct {
	Job    *batchv1.Job
	Pods   []corev1.Pod
	Events []corev1.Event
}

type clientGo struct {
	cs        *kubernetes.Clientset
	cfg       *rest.Config
	namespace string
	userLabel string
}

// New connects to the cluster (in-cluster, then KUBECONFIG env, then ~/.kube/config).
func New(namespace, userLabel string) (Client, error) {
	cfg, err := loadRestConfig()
	if err != nil {
		return nil, err
	}
	cfg.Timeout = 10 * time.Second
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("new clientset: %w", err)
	}
	return &clientGo{cs: cs, cfg: cfg, namespace: namespace, userLabel: userLabel}, nil
}

func loadRestConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("locate home: %w", err)
		}
		kubeconfig = home + "/.kube/config"
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %s: %w", kubeconfig, err)
	}
	return cfg, nil
}

func (c *clientGo) Namespace() string { return c.namespace }
func (c *clientGo) UserLabel() string { return c.userLabel }

func (c *clientGo) ListJobs(ctx context.Context) ([]model.Job, error) {
	jl, err := c.cs.BatchV1().Jobs(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	pl, err := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	podsByJob := map[string]int{}
	for i := range pl.Items {
		if owner := metav1.GetControllerOf(&pl.Items[i]); owner != nil && owner.Kind == "Job" {
			podsByJob[owner.Name]++
		}
	}
	out := make([]model.Job, 0, len(jl.Items))
	for i := range jl.Items {
		out = append(out, jobFromAPI(&jl.Items[i], podsByJob[jl.Items[i].Name], c.userLabel, c.namespace))
	}
	return out, nil
}

func (c *clientGo) GetJob(ctx context.Context, name string) (*batchv1.Job, error) {
	j, err := c.cs.BatchV1().Jobs(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get job %s: %w", name, err)
	}
	return j, nil
}

func (c *clientGo) GetJobYAML(ctx context.Context, name string) ([]byte, error) {
	j, err := c.GetJob(ctx, name)
	if err != nil {
		return nil, err
	}
	return marshalYAML(j)
}

func (c *clientGo) DeleteJob(ctx context.Context, name string) error {
	prog := metav1.DeletePropagationForeground
	if err := c.cs.BatchV1().Jobs(c.namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &prog,
	}); err != nil {
		return fmt.Errorf("delete job %s: %w", name, err)
	}
	return nil
}

func (c *clientGo) CreateJob(ctx context.Context, manifest []byte, dryRun bool) (*batchv1.Job, error) {
	dec := yaml.NewYAMLOrJSONDecoder(bytesReader(manifest), 4096)
	var job batchv1.Job
	if err := dec.Decode(&job); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if job.Kind != "" && job.Kind != "Job" {
		return nil, fmt.Errorf("manifest kind %q is not Job", job.Kind)
	}
	opts := metav1.CreateOptions{}
	if dryRun {
		opts.DryRun = []string{metav1.DryRunAll}
	}
	created, err := c.cs.BatchV1().Jobs(c.namespace).Create(ctx, &job, opts)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	return created, nil
}

// pickPodForJob returns the most useful pod for log/exec purposes: a Running
// pod if one exists, otherwise the most recently created pod. Returns an
// error if no pods exist for the job.
func (c *clientGo) pickPodForJob(ctx context.Context, jobName string) (*corev1.Pod, error) {
	pods, err := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil {
		return nil, fmt.Errorf("list pods for %s: %w", jobName, err)
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods found for job %s", jobName)
	}
	var running, latest *corev1.Pod
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.Status.Phase == corev1.PodRunning && running == nil {
			running = p
		}
		if latest == nil || p.CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = p
		}
	}
	if running != nil {
		return running, nil
	}
	return latest, nil
}

// JobLogs fetches a snapshot of the pod logs for jobName. tailLines limits the
// returned lines (0 means unlimited).
func (c *clientGo) JobLogs(ctx context.Context, name string, tailLines int64) ([]byte, error) {
	pod, err := c.pickPodForJob(ctx, name)
	if err != nil {
		return nil, err
	}
	opts := &corev1.PodLogOptions{}
	if tailLines > 0 {
		t := tailLines
		opts.TailLines = &t
	}
	req := c.cs.CoreV1().Pods(c.namespace).GetLogs(pod.Name, opts)
	rc, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("open logs for %s/%s: %w", name, pod.Name, err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// StreamJobLogs returns a follow-mode reader for the pod's logs. The caller
// must Close the returned reader to stop the stream.
func (c *clientGo) StreamJobLogs(ctx context.Context, name string, tailLines int64) (io.ReadCloser, error) {
	pod, err := c.pickPodForJob(ctx, name)
	if err != nil {
		return nil, err
	}
	opts := &corev1.PodLogOptions{Follow: true}
	if tailLines > 0 {
		t := tailLines
		opts.TailLines = &t
	}
	req := c.cs.CoreV1().Pods(c.namespace).GetLogs(pod.Name, opts)
	rc, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("stream logs for %s/%s: %w", name, pod.Name, err)
	}
	return rc, nil
}

// DescribeJob returns the live Job, all of its pods, and the merged event
// timeline (Job events + per-pod events).
func (c *clientGo) DescribeJob(ctx context.Context, name string) (*JobSnapshot, error) {
	job, err := c.GetJob(ctx, name)
	if err != nil {
		return nil, err
	}
	pl, err := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", name),
	})
	if err != nil {
		return nil, fmt.Errorf("list pods for %s: %w", name, err)
	}
	involved := []string{name}
	for i := range pl.Items {
		involved = append(involved, pl.Items[i].Name)
	}
	var events []corev1.Event
	for _, n := range involved {
		evs, err := c.cs.CoreV1().Events(c.namespace).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("involvedObject.name=%s", n),
		})
		if err != nil {
			continue // event read is best-effort
		}
		events = append(events, evs.Items...)
	}
	sortEventsByTime(events)
	return &JobSnapshot{Job: job, Pods: pl.Items, Events: events}, nil
}

func (c *clientGo) ExecJob(ctx context.Context, name string, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
	pod, err := c.pickPodForJob(ctx, name)
	if err != nil {
		return err
	}
	if pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("no running pods for job %s", name)
	}
	container := ""
	if len(pod.Spec.Containers) > 0 {
		container = pod.Spec.Containers[0].Name
	}
	req := c.cs.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(c.namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"/bin/bash"},
			Stdin:     stdin != nil,
			Stdout:    true,
			Stderr:    true,
			TTY:       tty,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(c.cfg, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("init exec: %w", err)
	}
	opts := remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    tty,
	}
	if tty {
		queue := newTerminalSizeQueue(ctx)
		opts.TerminalSizeQueue = queue
		defer queue.stop()
	}
	return executor.StreamWithContext(ctx, opts)
}
