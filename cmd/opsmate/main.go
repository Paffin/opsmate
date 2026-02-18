package main

import (
	"fmt"
	"os"

	"github.com/paffin/opsmate/internal/config"
	infractx "github.com/paffin/opsmate/internal/context"
	"github.com/paffin/opsmate/internal/launcher"
	"github.com/paffin/opsmate/internal/tui"
	dockermcp "github.com/paffin/opsmate/mcp/docker"
	filesmcp "github.com/paffin/opsmate/mcp/files"
	k8smcp "github.com/paffin/opsmate/mcp/kubernetes"
	prommcp "github.com/paffin/opsmate/mcp/prometheus"

	"github.com/spf13/cobra"
)

var (
	cfgFile  string
	readOnly bool
	modelArg string
	version  = "dev"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "opsmate",
		Short:   "DevOps AI Assistant powered by Claude Code",
		Long:    "opsmate gives Claude Code full understanding of your infrastructure through specialized MCP servers.",
		RunE:    runRoot,
		Version: version,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.opsmate/config.yaml)")
	rootCmd.Flags().BoolVar(&readOnly, "readonly", false, "read-only mode: disables apply, scale, delete, exec")
	rootCmd.Flags().StringVar(&modelArg, "model", "", "Claude model override (haiku/sonnet/opus or full model ID)")

	// MCP subcommands (used internally by .mcp.json)
	mcpCmd := &cobra.Command{
		Use:    "mcp",
		Short:  "Run MCP servers (used internally)",
		Hidden: true,
	}

	mcpCmd.AddCommand(mcpKubernetesCmd())
	mcpCmd.AddCommand(mcpDockerCmd())
	mcpCmd.AddCommand(mcpPrometheusCmd())
	mcpCmd.AddCommand(mcpFilesCmd())

	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(statusCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runRoot(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Apply --readonly flag: disables destructive operations on all servers
	if readOnly {
		cfg.Servers.Kubernetes.ReadOnly = true
		cfg.Servers.Docker.ReadOnly = true
	}

	// Build server description list for the banner
	var serverDescs []string
	if cfg.Servers.Kubernetes.Enabled {
		desc := "kubernetes"
		if cfg.Servers.Kubernetes.Context != "" {
			desc += fmt.Sprintf(" (context: %s)", cfg.Servers.Kubernetes.Context)
		}
		serverDescs = append(serverDescs, desc)
	}
	if cfg.Servers.Docker.Enabled {
		desc := "docker"
		if cfg.Servers.Docker.ReadOnly {
			desc += " (readonly)"
		}
		serverDescs = append(serverDescs, desc)
	}
	if cfg.Servers.Prometheus.Enabled {
		serverDescs = append(serverDescs, fmt.Sprintf("prometheus (%s)", cfg.Servers.Prometheus.URL))
	}
	if cfg.Servers.Files.Enabled {
		serverDescs = append(serverDescs, "file-analyzer")
	}

	// Setup MCP config and CLAUDE.md
	workDir, _ := os.Getwd()
	l := launcher.New(cfg, workDir)
	mcpConfigPath, _, cleanup, err := l.Setup()
	if err != nil {
		return err
	}
	defer cleanup()

	// Set model override if provided via --model flag
	if modelArg != "" {
		tui.ModelOverride = resolveModelAlias(modelArg)
	}

	// Run TUI with banner — banner is printed inside tui.Run
	return tui.Run(mcpConfigPath, workDir, serverDescs)
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show infrastructure status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}

			ctx := infractx.Collect(cfg)
			fmt.Println("  opsmate status")
			fmt.Println()
			fmt.Printf("  %s\n", ctx.Summary())
			return nil
		},
	}
}

// MCP server subcommands

func mcpKubernetesCmd() *cobra.Command {
	var kubeconfig, kubeContext string
	var readonly bool

	cmd := &cobra.Command{
		Use:   "kubernetes",
		Short: "Run Kubernetes MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load(cfgFile)
			if kubeconfig != "" {
				cfg.Servers.Kubernetes.Kubeconfig = kubeconfig
			}
			if kubeContext != "" {
				cfg.Servers.Kubernetes.Context = kubeContext
			}
			if cmd.Flags().Changed("readonly") {
				cfg.Servers.Kubernetes.ReadOnly = readonly
			}
			return k8smcp.Run(cfg.Servers.Kubernetes, cfg.Safety.MaxLogLines)
		},
	}

	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig")
	cmd.Flags().StringVar(&kubeContext, "context", "", "kubernetes context")
	cmd.Flags().BoolVar(&readonly, "readonly", false, "read-only mode")

	return cmd
}

func mcpDockerCmd() *cobra.Command {
	var host string
	var readonly bool

	cmd := &cobra.Command{
		Use:   "docker",
		Short: "Run Docker MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load(cfgFile)
			if host != "" {
				cfg.Servers.Docker.Host = host
			}
			if cmd.Flags().Changed("readonly") {
				cfg.Servers.Docker.ReadOnly = readonly
			}
			return dockermcp.Run(cfg.Servers.Docker, cfg.Safety.MaxLogLines)
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "Docker host")
	cmd.Flags().BoolVar(&readonly, "readonly", false, "read-only mode")

	return cmd
}

func mcpPrometheusCmd() *cobra.Command {
	var promURL, username, password string

	cmd := &cobra.Command{
		Use:   "prometheus",
		Short: "Run Prometheus MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load(cfgFile)
			if promURL != "" {
				cfg.Servers.Prometheus.URL = promURL
			}
			if username != "" {
				cfg.Servers.Prometheus.BasicAuth = &config.BasicAuth{
					Username: username,
					Password: password,
				}
			}
			return prommcp.Run(cfg.Servers.Prometheus)
		},
	}

	cmd.Flags().StringVar(&promURL, "url", "", "Prometheus URL")
	cmd.Flags().StringVar(&username, "username", "", "Basic auth username")
	cmd.Flags().StringVar(&password, "password", "", "Basic auth password")

	return cmd
}

// resolveModelAlias maps short aliases to full model IDs.
func resolveModelAlias(m string) string {
	switch m {
	case "haiku", "fast":
		return tui.ModelFast
	case "sonnet", "default":
		return tui.ModelDefault
	case "opus", "deep":
		return tui.ModelDeep
	default:
		return m // assume full model ID
	}
}

func mcpFilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Run File Analyzer MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load(cfgFile)
			return filesmcp.Run(cfg.Servers.Files)
		},
	}
	return cmd
}
