package engine

// GetRuntime creates and returns a ContainerRuntime instance based on the provided string.
func GetRuntime(runtimeType string, allowedTargets []string) (ContainerRuntime, error) {
	if runtimeType == "kubernetes" || runtimeType == "k8s" {
		return NewKubernetesClient(allowedTargets)
	}
	return NewDockerClient(allowedTargets)
}
