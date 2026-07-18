package engine

import "testing"

func TestLooksLikeProductionContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  string
		want bool
	}{
		{"empty", "", false},
		{"exact prod", "prod", true},
		{"exact production", "production", true},
		{"prefixed", "prod-us-east", true},
		{"suffixed", "acme-production", true},
		{"mixed case", "ACME-Production-Cluster", true},
		{"uppercase PROD", "PROD", true},
		{"staging", "staging", false},
		{"dev", "dev", false},
		{"minikube", "minikube", false},
		{"docker-desktop", "docker-desktop", false},
		{"kind-cluster", "kind-kind", false},
		{"unrelated cluster name", "acme-cluster", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeProductionContext(tt.ctx); got != tt.want {
				t.Errorf("looksLikeProductionContext(%q) = %v, want %v", tt.ctx, got, tt.want)
			}
		})
	}
}

func TestCheckProductionSafety_AllowsByDefault(t *testing.T) {
	if err := checkProductionSafety("staging"); err != nil {
		t.Errorf("expected no error for non-production context, got: %v", err)
	}
	if err := checkProductionSafety(""); err != nil {
		t.Errorf("expected no error for empty (undeterminable) context, got: %v", err)
	}
}

func TestCheckProductionSafety_BlocksProductionContextName(t *testing.T) {
	err := checkProductionSafety("acme-production")
	if err == nil {
		t.Fatal("expected an error for a context name that looks like production")
	}
}

func TestCheckProductionSafety_BlocksEnvironmentVar(t *testing.T) {
	t.Setenv("ENTROPY_ENVIRONMENT", "production")
	err := checkProductionSafety("some-safe-looking-context")
	if err == nil {
		t.Fatal("expected an error when ENTROPY_ENVIRONMENT=production, regardless of context name")
	}
}

func TestCheckProductionSafety_OverrideAllowsProductionContext(t *testing.T) {
	t.Setenv("ENTROPY_ALLOW_PRODUCTION", "true")
	if err := checkProductionSafety("acme-production"); err != nil {
		t.Errorf("expected ENTROPY_ALLOW_PRODUCTION=true to override the context-name check, got: %v", err)
	}
}

func TestCheckProductionSafety_OverrideAllowsEnvironmentVar(t *testing.T) {
	t.Setenv("ENTROPY_ENVIRONMENT", "production")
	t.Setenv("ENTROPY_ALLOW_PRODUCTION", "true")
	if err := checkProductionSafety(""); err != nil {
		t.Errorf("expected ENTROPY_ALLOW_PRODUCTION=true to override ENTROPY_ENVIRONMENT=production, got: %v", err)
	}
}
