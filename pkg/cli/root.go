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

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the entropy version",
	Run: func(cmd *cobra.Command, args []string) {
		// Use cmd.OutOrStdout() rather than fmt.Println so this respects
		// cobra's output redirection (SetOut), making it testable without
		// capturing the real os.Stdout.
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), Version)
	},
}

var runtimeType string

func init() {
	// rootCmd.Version enables cobra's built-in --version/-v flag; the
	// explicit `version` subcommand below covers the other common way
	// users check a CLI's version. Version is set at build time via
	// ldflags (see Makefile / .goreleaser.yaml); it defaults to "dev" for
	// `go run`/local builds.
	rootCmd.Version = Version
	rootCmd.AddCommand(versionCmd)

	rootCmd.PersistentFlags().StringVar(&runtimeType, "runtime", "docker", "Container runtime to use (docker, kubernetes)")
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// GetRootCommand returns the root cobra command for testing.
func GetRootCommand() *cobra.Command {
	return rootCmd
}
