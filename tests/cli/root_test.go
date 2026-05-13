package cli_test

import (
	"testing"

	"github.com/ibrahimkizilarslan/entropy/pkg/cli"
)

func TestRootCommand(t *testing.T) {
	rootCmd := cli.GetRootCommand()
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
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if (err != nil) != tt.wantErr {
				t.Errorf("rootCmd.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRootCommandDescription(t *testing.T) {
	rootCmd := cli.GetRootCommand()
	// Test root command structure
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
	rootCmd := cli.GetRootCommand()
	expectedCommands := []string{"init", "scenario", "start", "stop", "status", "logs", "cleanup", "topology", "doctor"}

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
