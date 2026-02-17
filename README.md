<p align="center">
  <img src="docs/assets/logo.png" alt="opsmate logo" width="200" />
</p>

<h1 align="center">opsmate</h1>

<p align="center">
  <strong>DevOps AI Assistant powered by Claude Code</strong><br>
  Give Claude Code full understanding of your infrastructure. One command.
</p>

<p align="center">
  <a href="https://github.com/YOUR_USERNAME/opsmate/releases"><img src="https://img.shields.io/github/v/release/YOUR_USERNAME/opsmate?style=flat-square" alt="Release"></a>
  <a href="https://github.com/YOUR_USERNAME/opsmate/actions"><img src="https://img.shields.io/github/actions/workflow/status/YOUR_USERNAME/opsmate/ci.yml?style=flat-square" alt="CI"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License"></a>
  <a href="https://goreportcard.com/report/github.com/YOUR_USERNAME/opsmate"><img src="https://goreportcard.com/badge/github.com/YOUR_USERNAME/opsmate?style=flat-square" alt="Go Report Card"></a>
</p>

<p align="center">
  <img src="docs/assets/demo.gif" alt="opsmate demo" width="720" />
</p>

---

## The Problem

You're debugging a production incident at 2 AM. You switch between `kubectl`, Prometheus, Docker logs, and ChatGPT — copy-pasting context back and forth. The AI doesn't know your cluster state. You waste time explaining what you're looking at.

## The Solution

**opsmate** launches specialized [MCP servers](https://modelcontextprotocol.io/) that connect Claude Code directly to your infrastructure. No copy-pasting. No context switching. Claude sees your pods, metrics, logs, and configs — and acts on them.

```bash
$ opsmate
🔧 Starting MCP servers...
  ✔ kubernetes (context: production, 47 pods)
  ✔ docker (23 containers)
  ✔ prometheus (http://prometheus:9090)
  ✔ file-analyzer (3 rulesets)

🚀 Launching Claude Code with DevOps superpowers...
```

Then just talk to it:

```
> Why is pod nginx-7b5f9 crashing?

I'll investigate the pod failure.

• Pod status: CrashLoopBackOff (restarted 7 times)
• Last log: "Killed" — OOMKilled
• Memory limit: 128Mi, actual usage: ~240Mi
• Node memory: 87% utilized

Root cause: The container exceeds its 128Mi memory limit.

Fix: kubectl set resources deployment/nginx --limits=memory=512Mi

Apply this fix? [y/N]
```

## Quick Start

### Install

```bash
# Homebrew (macOS/Linux)
brew install YOUR_USERNAME/tap/opsmate

# Go install
go install github.com/YOUR_USERNAME/opsmate/cmd/opsmate@latest

# Or download binary
curl -sSL https://raw.githubusercontent.com/YOUR_USERNAME/opsmate/main/scripts/install.sh | bash
```

### Prerequisites

- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) installed and authenticated
- Access to your infrastructure (kubeconfig, docker socket, prometheus URL)

### Run

```bash
# Auto-detect everything
opsmate

# Specify what to connect
opsmate --kube-context production --prometheus http://prom:9090

# Read-only mode (safe for production)
opsmate --readonly
```

### Configure

```bash
# Interactive setup
opsmate init

# Or edit manually
cat ~/.opsmate/config.yaml
```

## What Can It Do?

### 🐳 Kubernetes
Ask about pods, deployments, services, events, logs. Scale, restart, apply manifests — with confirmation prompts for destructive operations.

```
> Show me pods that are using more than 80% of their memory limit
> Why did the last deployment of auth-service fail?
> Scale the worker deployment to 5 replicas
```

### 🐋 Docker
Inspect containers, check resource usage, read logs, manage compose projects.

```
> Which container is using the most CPU?
> Show me the logs from the postgres container in the last hour
> What's different between the running config and the compose file?
```

### 📊 Prometheus
Query metrics, check alerts, investigate anomalies — in natural language, no PromQL needed.

```
> Are there any firing alerts?
> Show me the request latency trend for the API over the last 6 hours
> What's the error rate for the payment service?
```

### 📁 Infrastructure Files
Analyze Dockerfiles, Kubernetes YAML, docker-compose, Terraform — lint, validate, and get AI-powered improvement suggestions.

```
> Audit all Dockerfiles in this repo for security issues
> Is my Terraform config following best practices?
> Review the changes in my Kubernetes manifests before I apply them
```

## Architecture

```
                        ┌──────────────┐
                        │  opsmate CLI │
                        │   (Go bin)   │
                        └──────┬───────┘
                               │ manages
               ┌───────────────┼───────────────┐
               ▼               ▼               ▼
        ┌────────────┐ ┌────────────┐ ┌────────────┐
        │  K8s MCP   │ │ Docker MCP │ │  Prom MCP  │ ...
        │  Server    │ │  Server    │ │  Server    │
        └─────┬──────┘ └─────┬──────┘ └─────┬──────┘
              │              │              │
              ▼              ▼              ▼
         K8s Cluster    Docker Host    Prometheus
```

Each MCP server runs as a subprocess with **stdio transport** — no network ports, no API keys, no attack surface. Claude Code discovers tools through the standard MCP protocol.

## Safety

opsmate is designed to be safe by default:

- **`--readonly` mode** — disables all write operations (apply, scale, delete)
- **Confirmation prompts** — destructive operations require explicit `y` confirmation
- **Secret redaction** — passwords, tokens, and keys are masked in output
- **Audit log** — all operations are logged to `~/.opsmate/audit.log`
- **Namespace restrictions** — limit access to specific namespaces
- **Max log lines** — prevents memory issues from huge log outputs

## Configuration

<details>
<summary>Full config reference</summary>

```yaml
# ~/.opsmate/config.yaml

servers:
  kubernetes:
    enabled: true
    kubeconfig: ~/.kube/config
    context: ""                    # empty = current context
    namespaces: []                 # empty = all
    readonly: false

  docker:
    enabled: true
    host: unix:///var/run/docker.sock
    readonly: true

  prometheus:
    enabled: true
    url: http://localhost:9090

  files:
    enabled: true
    scan_paths: ["."]
    rulesets: [dockerfile, kubernetes, compose, terraform]

safety:
  confirm_destructive: true
  max_log_lines: 1000
  redact_secrets: true

claude:
  model: claude-sonnet-4-20250514
```

</details>

## vs. Alternatives

| | opsmate | kubectl + ChatGPT | k9s | Lens |
|---|:---:|:---:|:---:|:---:|
| AI-powered analysis | ✅ | Manual copy-paste | ❌ | ❌ |
| Live cluster access | ✅ | ❌ | ✅ | ✅ |
| Docker + Prometheus | ✅ | ❌ | ❌ | Plugin |
| File linting | ✅ | ❌ | ❌ | ❌ |
| Natural language | ✅ | ✅ | ❌ | ❌ |
| Safety guardrails | ✅ | — | ⚠️ | ⚠️ |
| Single binary | ✅ | — | ✅ | ❌ |

## Roadmap

- [x] Kubernetes MCP server
- [x] Docker MCP server
- [x] Prometheus MCP server
- [x] File analyzer with lint rules
- [ ] Terraform MCP server
- [ ] Ansible MCP server
- [ ] `opsmate doctor` — diagnose environment issues
- [ ] Plugin system for custom MCP servers
- [ ] Helm chart for in-cluster deployment
- [ ] Grafana MCP server
- [ ] CI/CD pipeline MCP (GitLab CI, GitHub Actions)

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](docs/contributing.md) for guidelines.

The easiest way to contribute is to add new MCP servers or lint rules. Each MCP server is self-contained in its own package under `mcp/`.

## License

[MIT](LICENSE) — use it however you want.

---

<p align="center">
  <strong>If opsmate saves you time during an incident, consider giving it a ⭐</strong>
</p>
