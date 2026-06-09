package registry

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Reverter is the minimal interface the recovery engine needs to revert chaos.
// Both DockerClient and KubernetesClient satisfy this interface.
// It is intentionally narrow to avoid an import cycle between pkg/registry and pkg/engine.
type Reverter interface {
	// ExecCommand runs a command inside the target container/pod.
	// Used to issue `tc qdisc del` for network chaos revert.
	ExecCommand(ctx context.Context, target string, cmd []string) (int, error)

	// RevertResources resets all CPU and memory limits on the target to unlimited.
	// Used to revert resource chaos injections.
	RevertResources(ctx context.Context, target string) error
}

// RecoveryResult holds the outcome of a single orphan revert attempt.
type RecoveryResult struct {
	RecordID string
	Target   string
	FaultType FaultType
	Reverted  bool
	Err       error
}

// RecoverOrphans scans the registry for active (non-reverted) faults whose targets
// are present in the allowedTargets list, and attempts to revert each one.
//
// Scope rule: only targets in allowedTargets are recovered. This prevents
// accidental revert of chaos injected by a different session or different config.
//
// It is designed to be called once at engine boot, before the main chaos loop begins.
// It logs all outcomes via slog and returns a summary of results.
func (r *FaultRegistry) RecoverOrphans(ctx context.Context, allowedTargets []string, reverter Reverter, netIface string) []RecoveryResult {
	allowed := make(map[string]bool, len(allowedTargets))
	for _, t := range allowedTargets {
		allowed[t] = true
	}

	active := r.ListActive()
	if len(active) == 0 {
		slog.Info("registry: no orphaned faults detected, proceeding normally")
		return nil
	}

	slog.Warn("registry: ORPHANED FAULTS DETECTED — beginning crash-recovery",
		slog.Int("total_active", len(active)),
	)

	var results []RecoveryResult

	for _, rec := range active {
		// Scope filter: only recover targets that are in the current config
		if !allowed[rec.Target] {
			slog.Warn("registry: skipping orphan (target not in current config)",
				slog.String("id", rec.ID),
				slog.String("target", rec.Target),
				slog.String("fault_type", string(rec.FaultType)),
			)
			continue
		}

		result := RecoveryResult{
			RecordID:  rec.ID,
			Target:    rec.Target,
			FaultType: rec.FaultType,
		}

		slog.Warn("registry: reverting orphaned fault",
			slog.String("id", rec.ID),
			slog.String("target", rec.Target),
			slog.String("fault_type", string(rec.FaultType)),
			slog.Time("injected_at", rec.InjectedAt),
		)

		err := revertFault(ctx, rec, reverter, netIface)
		if err != nil {
			slog.Error("registry: failed to revert orphaned fault",
				slog.String("id", rec.ID),
				slog.String("target", rec.Target),
				slog.String("error", err.Error()),
			)
			result.Err = err
			results = append(results, result)
			continue
		}

		// Mark as reverted in registry
		if markErr := r.MarkReverted(rec.ID); markErr != nil {
			slog.Error("registry: fault reverted but MarkReverted failed",
				slog.String("id", rec.ID),
				slog.String("error", markErr.Error()),
			)
			result.Err = markErr
		} else {
			result.Reverted = true
			slog.Info("registry: orphaned fault successfully reverted",
				slog.String("id", rec.ID),
				slog.String("target", rec.Target),
			)
		}

		results = append(results, result)
	}

	// Garbage collect successfully reverted records
	if n, err := r.GarbageCollect(); err != nil {
		slog.Warn("registry: post-recovery GC failed", slog.String("error", err.Error()))
	} else if n > 0 {
		slog.Info("registry: GC complete", slog.Int("records_removed", n))
	}

	return results
}

// revertFault dispatches the correct revert operation for a given FaultRecord.
// It uses the network interface from the record params if available, falling back
// to the provided netIface default.
func revertFault(ctx context.Context, rec FaultRecord, reverter Reverter, defaultIface string) error {
	// Use a short timeout for revert operations to avoid blocking boot
	revertCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	switch rec.FaultType {
	case FaultTypeNetworkDelay, FaultTypeNetworkLoss:
		return revertNetworkFault(revertCtx, rec, reverter, defaultIface)

	case FaultTypeCPULimit:
		return revertCPUFault(revertCtx, rec, reverter)

	case FaultTypeMemoryLimit:
		return revertMemoryFault(revertCtx, rec, reverter)

	default:
		return fmt.Errorf("unknown fault type %q — cannot revert", rec.FaultType)
	}
}

func revertNetworkFault(ctx context.Context, rec FaultRecord, reverter Reverter, defaultIface string) error {
	iface := defaultIface
	if v, ok := rec.Params["iface"].(string); ok && v != "" {
		iface = v
	}
	if iface == "" {
		iface = "eth0"
	}

	// Remove the tc qdisc rule entirely — idempotent even if already removed
	cmd := []string{"tc", "qdisc", "del", "dev", iface, "root"}
	exitCode, err := reverter.ExecCommand(ctx, rec.Target, cmd)
	if err != nil {
		return fmt.Errorf("tc revert exec failed: %w", err)
	}
	// Exit code 2 means "no such qdisc" — treat as success (already clean)
	if exitCode != 0 && exitCode != 2 {
		return fmt.Errorf("tc revert returned unexpected exit code %d", exitCode)
	}
	return nil
}

func revertCPUFault(ctx context.Context, rec FaultRecord, reverter Reverter) error {
	if err := reverter.RevertResources(ctx, rec.Target); err != nil {
		return fmt.Errorf("CPU limit revert failed: %w", err)
	}
	return nil
}

func revertMemoryFault(ctx context.Context, rec FaultRecord, reverter Reverter) error {
	if err := reverter.RevertResources(ctx, rec.Target); err != nil {
		return fmt.Errorf("memory limit revert failed: %w", err)
	}
	return nil
}
