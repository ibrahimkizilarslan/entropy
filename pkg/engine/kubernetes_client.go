package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

// KubernetesClient implements the ContainerRuntime interface for Kubernetes clusters.
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
			return nil, fmt.Errorf("could not find kubeconfig")
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ns := os.Getenv("ENTROPY_K8S_NAMESPACE")
	if ns == "" {
		ns = "default" // default namespace
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

// findPod finds a pod by label "app=name" or exact name prefix.
func (k *KubernetesClient) findPod(name string) (*corev1.Pod, error) {
	// Try label selector first
	pods, err := k.clientset.CoreV1().Pods(k.namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", name),
	})
	if err == nil && len(pods.Items) > 0 {
		return &pods.Items[0], nil
	}

	// Fallback to finding by name prefix
	pods, err = k.clientset.CoreV1().Pods(k.namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, p := range pods.Items {
		if strings.HasPrefix(p.Name, name) {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("pod not found for target: %s", name)
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

func (k *KubernetesClient) ListContainers(all bool) ([]ContainerInfo, error) {
	pods, err := k.clientset.CoreV1().Pods(k.namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
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

func (k *KubernetesClient) StopContainer(name string, timeout int) (*ContainerInfo, error) {
	if err := k.assertAllowed(name); err != nil {
		return nil, err
	}
	p, err := k.findPod(name)
	if err != nil {
		return nil, err
	}

	var gracePeriodSeconds int64 = int64(timeout)
	err = k.clientset.CoreV1().Pods(k.namespace).Delete(context.Background(), p.Name, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to delete pod %s: %w", p.Name, err)
	}

	info := k.mapPodToContainerInfo(p)
	info.Status = "terminating"
	return info, nil
}

func (k *KubernetesClient) RestartContainer(name string, timeout int) (*ContainerInfo, error) {
	// In K8s, restarting a pod is deleting it. The ReplicaSet recreates it.
	return k.StopContainer(name, timeout)
}

func (k *KubernetesClient) PauseContainer(name string) (*ContainerInfo, error) {
	return nil, fmt.Errorf("PauseContainer is not natively supported in Kubernetes API without CRI manipulation")
}

func (k *KubernetesClient) UnpauseContainer(name string) (*ContainerInfo, error) {
	return nil, fmt.Errorf("UnpauseContainer is not natively supported in Kubernetes API without CRI manipulation")
}

func (k *KubernetesClient) GetContainerPID(name string) (int, error) {
	return 0, fmt.Errorf("GetContainerPID is not supported in Kubernetes via standard API")
}

func (k *KubernetesClient) injectEphemeralNetshoot(pod *corev1.Pod) error {
	for _, ec := range pod.Spec.EphemeralContainers {
		if ec.Name == "chaos-netshoot" {
			return nil // Already injected
		}
	}

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
			TTY:   true,
			Stdin: true,
		},
	}

	pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, ec)
	_, err := k.clientset.CoreV1().Pods(k.namespace).UpdateEphemeralContainers(context.Background(), pod.Name, pod, metav1.UpdateOptions{})
	return err
}

func (k *KubernetesClient) InjectNetworkDelay(target string, latencyMs int, jitterMs int, duration *int) error {
	if err := k.assertAllowed(target); err != nil {
		return err
	}
	p, err := k.findPod(target)
	if err != nil {
		return err
	}

	if err := k.injectEphemeralNetshoot(p); err != nil {
		return fmt.Errorf("failed to inject ephemeral container: %w", err)
	}

	// Wait for container to be running
	// Normally we'd watch, but for simplicity we assume it starts quickly.
	// Actually, we can just attempt to exec the tc command in a retry loop.

	cmd := []string{"tc", "qdisc", "add", "dev", "eth0", "root", "netem", "delay", fmt.Sprintf("%dms", latencyMs)}
	if jitterMs > 0 {
		cmd = append(cmd, fmt.Sprintf("%dms", jitterMs))
	}

	// We need to execute this command INSIDE the ephemeral container, not the main one.
	// Let's create an ExecInContainer helper.
	_, err = k.execInContainer(p.Name, "chaos-netshoot", cmd)
	return err
}

func (k *KubernetesClient) InjectNetworkLoss(target string, lossPercent int, duration *int) error {
	if err := k.assertAllowed(target); err != nil {
		return err
	}
	p, err := k.findPod(target)
	if err != nil {
		return err
	}

	if err := k.injectEphemeralNetshoot(p); err != nil {
		return fmt.Errorf("failed to inject ephemeral container: %w", err)
	}

	cmd := []string{"tc", "qdisc", "add", "dev", "eth0", "root", "netem", "loss", fmt.Sprintf("%d%%", lossPercent)}
	_, err = k.execInContainer(p.Name, "chaos-netshoot", cmd)
	return err
}

func (k *KubernetesClient) execInContainer(podName string, containerName string, cmd []string) (int, error) {
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
		return -1, fmt.Errorf("failed to create spdy executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		// Ignore if it fails because it's already added (exit status 2 from tc)
		if strings.Contains(stderr.String(), "File exists") {
			return 0, nil
		}
		// If container isn't ready yet, it returns BadRequest
		return 1, fmt.Errorf("exec failed: %w. Stderr: %s", err, stderr.String())
	}

	return 0, nil
}

func (k *KubernetesClient) UpdateContainerResources(name string, cpuQuota int64, cpuPeriod int64, memLimit int64) (*ContainerInfo, error) {
	return nil, fmt.Errorf("In-place resource updates are generally not supported for Pods in standard Kubernetes API without alpha features")
}

func (k *KubernetesClient) ExecCommand(name string, cmd []string) (int, error) {
	if err := k.assertAllowed(name); err != nil {
		return -1, err
	}
	p, err := k.findPod(name)
	if err != nil {
		return -1, err
	}

	if len(p.Spec.Containers) == 0 {
		return -1, fmt.Errorf("pod %s has no containers", p.Name)
	}

	containerName := p.Spec.Containers[0].Name
	return k.execInContainer(p.Name, containerName, cmd)
}

func (k *KubernetesClient) Close() {
	// client-go doesn't require explicit closing like docker client
}
