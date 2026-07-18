package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveNamespace(t *testing.T) {
	tests := []struct {
		name       string
		explicit   string
		envNs      string
		wantResult string
	}{
		{"explicit wins", "custom-ns", "env-ns", "custom-ns"},
		{"falls back to env var", "", "env-ns", "env-ns"},
		{"falls back to default", "", "", "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envNs != "" {
				t.Setenv("ENTROPY_K8S_NAMESPACE", tt.envNs)
			}
			if got := resolveNamespace(tt.explicit); got != tt.wantResult {
				t.Errorf("resolveNamespace(%q) = %q, want %q", tt.explicit, got, tt.wantResult)
			}
		})
	}
}

// writeTestKubeconfig writes a minimal, syntactically valid kubeconfig file
// with the given current-context name and returns its path.
func writeTestKubeconfig(t *testing.T, contextName string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	content := `
apiVersion: v1
kind: Config
current-context: ` + contextName + `
clusters:
- name: test-cluster
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: ` + contextName + `
  context:
    cluster: test-cluster
    user: test-user
users:
- name: test-user
  user:
    token: fake-token
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test kubeconfig: %v", err)
	}
	return path
}

// TestBuildK8sConfig_ReturnsContextName is a regression test for the P1
// production-safety fix: buildK8sConfig must surface the active kubeconfig
// context name so callers (NewKubernetesClient) can check it against
// looksLikeProductionContext. This test does not require a live cluster —
// BuildConfigFromFlags only parses the kubeconfig file, it doesn't dial out.
func TestBuildK8sConfig_ReturnsContextName(t *testing.T) {
	kubeconfigPath := writeTestKubeconfig(t, "acme-production")
	t.Setenv("KUBECONFIG", kubeconfigPath)

	_, contextName, err := buildK8sConfig()
	if err != nil {
		t.Fatalf("buildK8sConfig failed: %v", err)
	}
	if contextName != "acme-production" {
		t.Errorf("expected context name %q, got %q", "acme-production", contextName)
	}
}

func TestBuildK8sConfig_NonProductionContextName(t *testing.T) {
	kubeconfigPath := writeTestKubeconfig(t, "minikube")
	t.Setenv("KUBECONFIG", kubeconfigPath)

	_, contextName, err := buildK8sConfig()
	if err != nil {
		t.Fatalf("buildK8sConfig failed: %v", err)
	}
	if looksLikeProductionContext(contextName) {
		t.Errorf("expected context %q to not look like production", contextName)
	}
}
