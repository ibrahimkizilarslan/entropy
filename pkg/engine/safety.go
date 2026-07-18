package engine

import (
	"fmt"
	"os"
	"strings"
)

// looksLikeProductionContext reports whether a runtime context name (Docker
// context or Kubernetes kubeconfig context) suggests it targets a production
// environment. This is a heuristic, not a guarantee: it matches "prod" or
// "production" as a case-insensitive substring, which catches common naming
// conventions (e.g. "prod-us-east", "acme-production") without requiring an
// exact, brittle name list.
func looksLikeProductionContext(name string) bool {
	if name == "" {
		return false
	}
	lower := strings.ToLower(name)
	return strings.Contains(lower, "production") || strings.Contains(lower, "prod")
}

// checkProductionSafety refuses to proceed if either:
//   - ENTROPY_ENVIRONMENT is explicitly set to "production" (kept for
//     backwards compatibility with existing deployments that already set
//     this), or
//   - the active runtime context name looks like a production environment
//     (see looksLikeProductionContext).
//
// Both checks are bypassed by setting ENTROPY_ALLOW_PRODUCTION=true.
//
// contextName may be empty when it cannot be determined (e.g. in-cluster
// Kubernetes config has no concept of a "context", or DOCKER_HOST is used
// directly), in which case only the ENTROPY_ENVIRONMENT check applies —
// this is a heuristic, not a substitute for operators explicitly scoping
// where Entropy is allowed to run.
func checkProductionSafety(contextName string) error {
	if os.Getenv("ENTROPY_ALLOW_PRODUCTION") == "true" {
		return nil
	}

	if os.Getenv("ENTROPY_ENVIRONMENT") == "production" {
		return fmt.Errorf(
			"refusing to run: ENTROPY_ENVIRONMENT=production\n" +
				"  → Set ENTROPY_ALLOW_PRODUCTION=true to override",
		)
	}

	if looksLikeProductionContext(contextName) {
		return fmt.Errorf(
			"refusing to run: active context %q looks like a production environment\n"+
				"  → Set ENTROPY_ALLOW_PRODUCTION=true to override if this is intentional",
			contextName,
		)
	}

	return nil
}
