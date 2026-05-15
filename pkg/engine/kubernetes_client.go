package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

// KubernetesClient implements the ContainerRuntime interface for Kubernetes clusters.
//
// Supported actions:
//   - stop      → deletes the pod (ReplicaSet/Deployment recreates it)
//   - restart   → same as stop (delete + recreate by controller)
//   - delay     → injects tc/netem via an ephemeral netshoot sidecar
//   - loss      → injects tc/netem via an ephemeral netshoot sidecar
//   - exec      → runs a command in the first container of the pod
//   - limit_cpu → patches the pod's container resource limits via JSON patch
//
// Unsupported actions (return clear errors):
//   - pause     → requires CRI-level SIGSTOP, not available via K8s API
//   - unpause   → same as pause
//   - GetContainerPID → not available via standard K8s API
type KubernetesClient struct {
	clientset      *kubernetes.Clientset
	config         *rest.Config
	namespace      string
	allowedTargets map[string]bool
}

func NewKubernetesClient(allowedTargets []string) (*KubernetesClient, error) {
	// 0. Safety: prevent accidental use in production environments
	if os.Getenv("ENTROPY_ENVIRONMENT") == "production" && os.Getenv("ENTROPY_ALLOW_PRODUCTION") != "true" {
		return nil, fmt.Errorf("refusing to run in production environment. Set ENTROPY_ALLOW_PRODUCTION=true to override")
	}

	var config *rest.Config
	var err error

	// Try in-cluster config first
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			if home := homedir.HomeDir(); home != "" {
				kubeconfig = filepath.Join(home, ".kube", "config")
			}
		}

		if kubeconfig != "" {
			config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
			if err != nil {
				return nil, fmt.Errorf("failed to build kubeconfig from %s: %w", kubeconfig, err)
			}
		} else {
			return nil, fmt.Errorf("could not find kubeconfig. Set KUBECONFIG env or create ~/.kube/config")
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ns := os.Getenv("ENTROPY_K8S_NAMESPACE")
	if ns == "" {
		ns = "default"
	}

	var allowed map[string]bool
	if allowedTargets != nil {
		allowed = make(map[string]bool)
		for _, t := range allowedTargets {
			allowed[t] = true
		}
	}

	return &KubernetesClient{
		clientset:      clientset,
		config:         config,
		namespace:      ns,
		allowedTargets: allowed,
	}, nil
}

func (k *KubernetesClient) assertAllowed(name string) error {
	if k.allowedTargets != nil && !k.allowedTargets[name] {
		return fmt.Errorf("'%s' is not in the configured chaos targets", name)
	}
	return nil
}

// findPod finds a pod by label "app=name" or name prefix, respecting context cancellation.
func (k *KubernetesClient) findPod(ctx context.Context, name string) (*corev1.Pod, error) {
	// Try label selector first (most reliable for Deployments/StatefulSets)
	pods, err := k.clientset.CoreV1().Pods(k.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", name),
	})
	if err == nil && len(pods.Items) > 0 {
		// Prefer Running pods
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning && p.DeletionTimestamp == nil {
				return &p, nil
			}
		}
		return &pods.Items[0], nil
	}

	// Fallback: find by name prefix
	pods, err = k.clientset.CoreV1().Pods(k.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace '%s': %w", k.namespace, err)
	}
	for i, p := range pods.Items {
		if strings.HasPrefix(p.Name, name) && p.DeletionTimestamp == nil {
			return &pods.Items[i], nil
		}
	}

	return nil, fmt.Errorf("no running pod found for target '%s' in namespace '%s'", name, k.namespace)
}

func (k *KubernetesClient) mapPodToContainerInfo(p *corev1.Pod) *ContainerInfo {
	status := string(p.Status.Phase)
	if p.DeletionTimestamp != nil {
		status = "terminating"
	}

	image := ""
	if len(p.Spec.Containers) > 0 {
		image = p.Spec.Containers[0].Image
	}

	ports := make(map[string]string)
	for _, c := range p.Spec.Containers {
		for _, port := range c.Ports {
			ports[fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol)] = fmt.Sprintf("%d", port.HostPort)
		}
	}

	return &ContainerInfo{
		ID:     string(p.UID)[:12],
		Name:   p.Name,
		Image:  image,
		Status: status,
		Ports:  ports,
	}
}

func (k *KubernetesClient) ListContainers(ctx context.Context, all bool) ([]ContainerInfo, error) {
	pods, err := k.clientset.CoreV1().Pods(k.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace '%s': %w", k.namespace, err)
	}

	var res []ContainerInfo
	for _, p := range pods.Items {
		if !all && p.Status.Phase != corev1.PodRunning {
			continue
		}
		res = append(res, *k.mapPodToContainerInfo(&p))
	}
	return res, nil
}

func (k *KubernetesClient) StopContainer(ctx context.Context, name string, timeout int) (*ContainerInfo, error) {
	if err := k.assertAllowed(name); err != nil {
		return nil, err
	}
	p, err := k.findPod(ctx, name)
	if err != nil {
		return nil, err
	}

	var gracePeriodSeconds int64 = int64(timeout)
	err = k.clientset.CoreV1().Pods(k.namespace).Delete(ctx, p.Name, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to delete pod %s: %w", p.Name, err)
	}

	info := k.mapPodToContainerInfo(p)
	info.Status = "terminating"
	return info, nil
}

func (k *KubernetesClient) RestartContainer(ctx context.Context, name string, timeout int) (*ContainerInfo, error) {
	// In K8s, "restarting" a pod means deleting it. The owning controller (Deployment/ReplicaSet)
	// automatically creates a new replacement pod.
	return k.StopContainer(ctx, name, timeout)
}

// PauseContainer is not supported in Kubernetes via the standard API.
// Pausing a container requires sending SIGSTOP to the container process via the CRI (containerd/cri-o),
// which is not exposed through the Kubernetes API server.
func (k *KubernetesClient) PauseContainer(ctx context.Context, name string) (*ContainerInfo, error) {
	return nil, fmt.Errorf(
		"pause is not supported in Kubernetes runtime\n" +
			"  → Reason: pausing requires CRI-level SIGSTOP which is not available via the Kubernetes API\n" +
			"  → Alternative: use 'stop' action to delete the pod (controller will recreate it)",
	)
}

// UnpauseContainer is not supported in Kubernetes via the standard API.
func (k *KubernetesClient) UnpauseContainer(ctx context.Context, name string) (*ContainerInfo, error) {
	return nil, fmt.Errorf(
		"unpause is not supported in Kubernetes runtime\n" +
			"  → Reason: pausing requires CRI-level SIGSTOP which is not available via the Kubernetes API",
	)
}

// GetContainerPID is not supported in Kubernetes via the standard API.
func (k *KubernetesClient) GetContainerPID(ctx context.Context, name string) (int, error) {
	return 0, fmt.Errorf(
		"GetContainerPID is not supported in Kubernetes via the standard API\n" +
			"  → Use ExecCommand to run 'sh -c echo $$ ' inside the container if needed",
	)
}

// injectEphemeralNetshoot adds a netshoot ephemeral container to the pod for network chaos injection.
// It waits up to 30 seconds for the container to become Running before returning.
func (k *KubernetesClient) injectEphemeralNetshoot(ctx context.Context, pod *corev1.Pod) error {
	// Check if already injected
	for _, ec := range pod.Spec.EphemeralContainers {
		if ec.Name == "chaos-netshoot" {
			// Verify it's actually running
			for _, ecStatus := range pod.Status.EphemeralContainerStatuses {
				if ecStatus.Name == "chaos-netshoot" && ecStatus.State.Running != nil {
					return nil
				}
			}
			// It exists but isn't Running yet — fall through to wait logic below
		}
	}

	// Build the ephemeral container spec
	ec := corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:            "chaos-netshoot",
			Image:           "nicolaka/netshoot:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{"NET_ADMIN"},
				},
			},
			TTY:   false,
			Stdin: false,
			Command: []string{"sh", "-c", "sleep 3600"}, // Keep alive for chaos window
		},
	}

	pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, ec)
	_, err := k.clientset.CoreV1().Pods(k.namespace).UpdateEphemeralContainers(ctx, pod.Name, pod, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to inject ephemeral netshoot container into pod '%s': %w", pod.Name, err)
	}

	// Wait for the ephemeral container to become Running (up to 30 seconds)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for chaos-netshoot to start: %w", ctx.Err())
		default:
		}

		updated, err := k.clientset.CoreV1().Pods(k.namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to poll ephemeral container status for pod '%s': %w", pod.Name, err)
		}

		for _, ecStatus := range updated.Status.EphemeralContainerStatuses {
			if ecStatus.Name == "chaos-netshoot" && ecStatus.State.Running != nil {
				return nil // Ready!
			}
			if ecStatus.State.Waiting != nil && ecStatus.State.Waiting.Reason == "ErrImagePull" {
				return fmt.Errorf("failed to pull chaos-netshoot image: %s", ecStatus.State.Waiting.Message)
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout: chaos-netshoot container in pod '%s' did not become Running within 30s", pod.Name)
}

func (k *KubernetesClient) InjectNetworkDelay(ctx context.Context, target string, latencyMs int, jitterMs int, duration *int) error {
	if err := k.assertAllowed(target); err != nil {
		return err
	}
	p, err := k.findPod(ctx, target)
	if err != nil {
		return err
	}

	if err := k.injectEphemeralNetshoot(ctx, p); err != nil {
		return fmt.Errorf("failed to inject ephemeral container: %w", err)
	}

	cmd := []string{"tc", "qdisc", "add", "dev", "eth0", "root", "netem", "delay", fmt.Sprintf("%dms", latencyMs)}
	if jitterMs > 0 {
		cmd = append(cmd, fmt.Sprintf("%dms", jitterMs), "distribution", "normal")
	}
	_, err = k.execInContainer(ctx, p.Name, "chaos-netshoot", cmd)
	return err
}

func (k *KubernetesClient) InjectNetworkLoss(ctx context.Context, target string, lossPercent int, duration *int) error {
	if err := k.assertAllowed(target); err != nil {
		return err
	}
	p, err := k.findPod(ctx, target)
	if err != nil {
		return err
	}

	if err := k.injectEphemeralNetshoot(ctx, p); err != nil {
		return fmt.Errorf("failed to inject ephemeral container: %w", err)
	}

	cmd := []string{"tc", "qdisc", "add", "dev", "eth0", "root", "netem", "loss", fmt.Sprintf("%d%%", lossPercent)}
	_, err = k.execInContainer(ctx, p.Name, "chaos-netshoot", cmd)
	return err
}

func (k *KubernetesClient) execInContainer(ctx context.Context, podName string, containerName string, cmd []string) (int, error) {
	req := k.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(k.namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   cmd,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(k.config, "POST", req.URL())
	if err != nil {
		return -1, fmt.Errorf("failed to create spdy executor for pod '%s': %w", podName, err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		// tc returns "RTNETLINK answers: File exists" when rule is already set — treat as idempotent success
		if strings.Contains(stderr.String(), "File exists") {
			return 0, nil
		}
		return 1, fmt.Errorf("exec failed in pod '%s' container '%s': %w. Stderr: %s", podName, containerName, err, stderr.String())
	}

	return 0, nil
}

// UpdateContainerResources applies resource limits to the first container of a pod via JSON patch.
// Note: cpuPeriod is ignored for K8s (uses millicores via cpuQuota/1000).
// Only memory and CPU limits are patched; requests are left unchanged.
func (k *KubernetesClient) UpdateContainerResources(ctx context.Context, name string, cpuQuota int64, cpuPeriod int64, memLimit int64) (*ContainerInfo, error) {
	if err := k.assertAllowed(name); err != nil {
		return nil, err
	}
	p, err := k.findPod(ctx, name)
	if err != nil {
		return nil, err
	}

	if len(p.Spec.Containers) == 0 {
		return nil, fmt.Errorf("pod '%s' has no containers to patch", p.Name)
	}

	// Build a JSON merge patch for resource limits on the first container.
	// cpuQuota is in microseconds (Docker convention), convert to millicores.
	// Example: cpuQuota=50000, cpuPeriod=100000 → 500m
	patches := []string{}
	if cpuQuota > 0 && cpuPeriod > 0 {
		milliCPU := (cpuQuota * 1000) / cpuPeriod
		patches = append(patches, fmt.Sprintf(`"cpu":"%dm"`, milliCPU))
	} else if cpuQuota == 0 {
		patches = append(patches, `"cpu":null`)
	}

	if memLimit > 0 {
		patches = append(patches, fmt.Sprintf(`"memory":"%d"`, memLimit))
	} else if memLimit == 0 && cpuQuota == 0 {
		// Restoring: clear both
		patches = append(patches, `"memory":null`)
		patches = append(patches, `"cpu":null`)
	}

	if len(patches) == 0 {
		return k.mapPodToContainerInfo(p), nil
	}

	patchStr := fmt.Sprintf(`{"spec":{"containers":[{"name":"%s","resources":{"limits":{%s}}}]}}`,
		p.Spec.Containers[0].Name,
		strings.Join(patches, ","),
	)

	_, err = k.clientset.CoreV1().Pods(k.namespace).Patch(
		ctx, p.Name, types.StrategicMergePatchType, []byte(patchStr), metav1.PatchOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to patch resources for pod '%s': %w\n  → Note: in-place resource updates require 'InPlacePodVerticalScaling' feature gate enabled on the cluster", p.Name, err)
	}

	return k.mapPodToContainerInfo(p), nil
}

func (k *KubernetesClient) ExecCommand(ctx context.Context, name string, cmd []string) (int, error) {
	if err := k.assertAllowed(name); err != nil {
		return -1, err
	}
	p, err := k.findPod(ctx, name)
	if err != nil {
		return -1, err
	}

	if len(p.Spec.Containers) == 0 {
		return -1, fmt.Errorf("pod '%s' has no containers", p.Name)
	}

	containerName := p.Spec.Containers[0].Name
	return k.execInContainer(ctx, p.Name, containerName, cmd)
}

func (k *KubernetesClient) Close() {
	// client-go doesn't require explicit closing like docker client
}
