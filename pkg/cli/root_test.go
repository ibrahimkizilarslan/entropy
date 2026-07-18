package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "Help flag",
			args:    []string{"--help"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "entropy"}
			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			if (err != nil) != tt.wantErr {
				t.Errorf("rootCmd.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRootCommandDescription(t *testing.T) {
	if rootCmd.Short == "" {
		t.Error("rootCmd.Short should not be empty")
	}

	if rootCmd.Long == "" {
		t.Error("rootCmd.Long should not be empty")
	}

	expectedUse := "entropy"
	if rootCmd.Use != expectedUse {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, expectedUse)
	}
}

func TestRootCommandHasSubcommands(t *testing.T) {
	expectedCommands := []string{"init", "scenario", "start", "stop", "status", "logs", "cleanup", "topology", "doctor", "version"}

	for _, expectedCmd := range expectedCommands {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == expectedCmd {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Expected subcommand %q not found", expectedCmd)
		}
	}
}

// TestRootCommand_VersionIsWired verifies rootCmd.Version is set from the
// package-level Version variable, enabling cobra's built-in --version flag.
func TestRootCommand_VersionIsWired(t *testing.T) {
	if rootCmd.Version != Version {
		t.Errorf("rootCmd.Version = %q, want it wired to package Version %q", rootCmd.Version, Version)
	}
}

// TestVersionFlag_PrintsVersion drives the actual --version flag through
// rootCmd.Execute() and checks the configured Version string appears in the
// output, so a future refactor that breaks the wiring (e.g. rootCmd.Version
// no longer set, or the flag disabled) fails a test instead of only being
// caught by manually running the binary.
func TestVersionFlag_PrintsVersion(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"--version"})
	defer rootCmd.SetArgs(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute() with --version failed: %v", err)
	}
	if !strings.Contains(out.String(), Version) {
		t.Errorf("expected --version output to contain %q, got: %q", Version, out.String())
	}
}

// TestVersionCmdRun_PrintsVersion drives the `entropy version` subcommand's
// Run closure directly and checks it prints the configured Version.
func TestVersionCmdRun_PrintsVersion(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"version"})
	defer rootCmd.SetArgs(nil)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute() with 'version' failed: %v", err)
	}
	if !strings.Contains(out.String(), Version) {
		t.Errorf("expected 'version' subcommand output to contain %q, got: %q", Version, out.String())
	}
}
