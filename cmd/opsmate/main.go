package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/paffin/opsmate/internal/chatui"
	"github.com/paffin/opsmate/internal/config"
	infractx "github.com/paffin/opsmate/internal/context"
	"github.com/paffin/opsmate/internal/launcher"
	"github.com/paffin/opsmate/internal/mcphost"
	dockermcp "github.com/paffin/opsmate/mcp/docker"
	filesmcp "github.com/paffin/opsmate/mcp/files"
	k8smcp "github.com/paffin/opsmate/mcp/kubernetes"
	prommcp "github.com/paffin/opsmate/mcp/prometheus"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	version    = "dev"
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))
	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))
	serverStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "opsmate",
		Short: "DevOps AI Assistant powered by Claude Code",
		Long:  "opsmate gives Claude Code full understanding of your infrastructure through specialized MCP servers.",
		RunE:  runRoot,
		Version: version,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.opsmate/config.yaml)")

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
	rootCmd.AddCommand(chatCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runRoot(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Print banner
	fmt.Println(titleStyle.Render("  opsmate — DevOps AI Assistant"))
	fmt.Println()

	// Show enabled servers
	fmt.Println("  MCP Servers:")
	if cfg.Servers.Kubernetes.Enabled {
		ctx := ""
		if cfg.Servers.Kubernetes.Context != "" {
			ctx = fmt.Sprintf(" (context: %s)", cfg.Servers.Kubernetes.Context)
		}
		fmt.Printf("  %s kubernetes%s\n", successStyle.Render("✔"), ctx)
	}
	if cfg.Servers.Docker.Enabled {
		mode := ""
		if cfg.Servers.Docker.ReadOnly {
			mode = " (readonly)"
		}
		fmt.Printf("  %s docker%s\n", successStyle.Render("✔"), mode)
	}
	if cfg.Servers.Prometheus.Enabled {
		fmt.Printf("  %s prometheus (%s)\n", successStyle.Render("✔"), cfg.Servers.Prometheus.URL)
	}
	if cfg.Servers.Files.Enabled {
		fmt.Printf("  %s file-analyzer\n", successStyle.Render("✔"))
	}
	fmt.Println()

	// Launch
	workDir, _ := os.Getwd()
	l := launcher.New(cfg, workDir)
	fmt.Println(serverStyle.Render("  Launching Claude Code with DevOps superpowers..."))
	fmt.Println()

	return l.Launch()
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
			fmt.Println(titleStyle.Render("  opsmate status"))
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
			if readonly {
				cfg.Servers.Kubernetes.ReadOnly = true
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
			if readonly {
				cfg.Servers.Docker.ReadOnly = true
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

func chatCmd() *cobra.Command {
	var port int
	var noOpen bool

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Start the web-based Chat UI for Claude Code",
		Long:  "Starts a local HTTP server with a web chat interface that proxies queries to Claude Code with MCP DevOps context.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Override port from config if flag not explicitly set
			if !cmd.Flags().Changed("port") && cfg.Chat.Port != 0 {
				port = cfg.Chat.Port
			}

			workDir, _ := os.Getwd()

			// Generate .mcp.json for the chat session
			manager := mcphost.New(cfg)
			mcpConfigPath, err := manager.GenerateMCPConfig(workDir)
			if err != nil {
				return fmt.Errorf("generate mcp config: %w", err)
			}
			defer manager.Cleanup(workDir)

			// Print banner with enabled MCP servers
			fmt.Println(titleStyle.Render("  opsmate chat — Web Chat UI"))
			fmt.Println()
			fmt.Println("  MCP Servers:")
			if cfg.Servers.Kubernetes.Enabled {
				fmt.Printf("  %s kubernetes\n", successStyle.Render("✔"))
			}
			if cfg.Servers.Docker.Enabled {
				fmt.Printf("  %s docker\n", successStyle.Render("✔"))
			}
			if cfg.Servers.Prometheus.Enabled {
				fmt.Printf("  %s prometheus\n", successStyle.Render("✔"))
			}
			if cfg.Servers.Files.Enabled {
				fmt.Printf("  %s file-analyzer\n", successStyle.Render("✔"))
			}
			fmt.Println()

			url := fmt.Sprintf("http://localhost:%d", port)
			fmt.Printf("  %s\n\n", serverStyle.Render("Chat UI available at "+url))

			if !noOpen {
				openBrowser(url)
			}

			srv := chatui.New(port, workDir, mcpConfigPath)
			return srv.Start()
		},
	}

	cmd.Flags().IntVar(&port, "port", 3333, "HTTP port to listen on")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Do not automatically open the browser")

	return cmd
}

// openBrowser opens the given URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
