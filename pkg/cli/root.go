package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "entropy",
	Short: "A local chaos engineering toolkit for Docker-based microservices",
	Long: `Entropy is a developer-first chaos engineering platform designed to inject controlled faults into local microservice environments.

By prioritizing the developer workflow, Entropy enables teams to validate system resilience, identify single points of failure, and confidently test hypothesis-driven scenarios before code reaches production.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// GetRootCommand returns the root cobra command for testing
func GetRootCommand() *cobra.Command {
	return rootCmd
}
