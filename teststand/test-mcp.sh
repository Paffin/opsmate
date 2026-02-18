#!/bin/bash
# MCP Tool Test Runner
# Sends JSON-RPC calls to opsmate MCP servers and validates responses

OPSMATE="../opsmate-test.exe"
PASS=0
FAIL=0
TOTAL=0

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

call_mcp() {
  local server="$1"
  local tool="$2"
  local args="$3"
  local extra_args="$4"

  local init='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'
  local initialized='{"jsonrpc":"2.0","method":"notifications/initialized"}'
  local call="{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"${tool}\",\"arguments\":${args}}}"

  printf '%s\n%s\n%s\n' "$init" "$initialized" "$call" | $OPSMATE mcp $server $extra_args 2>/dev/null | tail -1
}

check_result() {
  local test_name="$1"
  local result="$2"
  local expect_text="$3"

  TOTAL=$((TOTAL + 1))

  if [ -z "$result" ]; then
    FAIL=$((FAIL + 1))
    printf "${RED}FAIL${NC} %s - empty response\n" "$test_name"
    return
  fi

  # Check for error
  local has_error=$(echo "$result" | python -c "import sys,json; d=json.load(sys.stdin); print('yes' if 'error' in d or d.get('result',{}).get('isError',False) else 'no')" 2>/dev/null)

  if [ "$has_error" = "yes" ]; then
    FAIL=$((FAIL + 1))
    printf "${RED}FAIL${NC} %s - error in response\n" "$test_name"
    echo "  Response: $(echo "$result" | head -c 200)"
    return
  fi

  # Check for expected text if provided
  if [ -n "$expect_text" ]; then
    local has_text=$(echo "$result" | python -c "import sys,json; d=json.load(sys.stdin); text=d.get('result',{}).get('content',[{}])[0].get('text',''); print('yes' if '$expect_text' in text else 'no')" 2>/dev/null)
    if [ "$has_text" != "yes" ]; then
      FAIL=$((FAIL + 1))
      printf "${RED}FAIL${NC} %s - missing expected text '%s'\n" "$test_name" "$expect_text"
      echo "  Response: $(echo "$result" | python -c "import sys,json; d=json.load(sys.stdin); print(d.get('result',{}).get('content',[{}])[0].get('text','')[:300])" 2>/dev/null)"
      return
    fi
  fi

  PASS=$((PASS + 1))
  printf "${GREEN}PASS${NC} %s\n" "$test_name"
}

echo "========================================"
echo "  opsmate MCP Integration Tests"
echo "========================================"
echo ""

# ==========================================
# DOCKER MCP SERVER TESTS
# ==========================================
echo -e "${YELLOW}--- Docker MCP Server ---${NC}"

result=$(call_mcp docker docker_ps '{"all":true}')
check_result "docker_ps (all containers)" "$result" "containers"

result=$(call_mcp docker docker_ps '{"all":false,"filter":"name=teststand"}')
check_result "docker_ps (with filter)" "$result" "containers"

# Get a container name for further tests
CONTAINER=$(docker ps --format '{{.Names}}' | grep teststand-nginx | head -1)

result=$(call_mcp docker docker_logs "{\"container\":\"$CONTAINER\",\"tail\":10}")
check_result "docker_logs (nginx)" "$result" "Logs for"

result=$(call_mcp docker docker_inspect "{\"container\":\"$CONTAINER\"}")
check_result "docker_inspect (nginx)" "$result" "Container"

result=$(call_mcp docker docker_stats "{\"container\":\"$CONTAINER\"}")
check_result "docker_stats (single)" "$result" "Stats for"

result=$(call_mcp docker docker_stats '{}')
check_result "docker_stats (all)" "$result" "Container stats"

result=$(call_mcp docker docker_images '{}')
check_result "docker_images" "$result" "images"

result=$(call_mcp docker docker_compose_ps '{}')
check_result "docker_compose_ps" "$result" "Compose services"

result=$(call_mcp docker docker_compose_logs '{"service":"nginx"}')
check_result "docker_compose_logs" "$result" "Compose logs"

# docker_exec (readonly by default, should fail)
result=$(call_mcp docker docker_exec "{\"container\":\"$CONTAINER\",\"command\":\"echo hello\"}")
check_result_exec() {
  TOTAL=$((TOTAL + 1))
  local has_readonly=$(echo "$result" | grep -c "read-only" 2>/dev/null)
  if [ "$has_readonly" -gt 0 ] || echo "$result" | python -c "import sys,json; d=json.load(sys.stdin); t=d.get('result',{}).get('content',[{}])[0].get('text',''); print('yes' if 'read-only' in t else 'no')" 2>/dev/null | grep -q "yes"; then
    PASS=$((PASS + 1))
    printf "${GREEN}PASS${NC} docker_exec (correctly blocked in readonly)\n"
  else
    FAIL=$((FAIL + 1))
    printf "${RED}FAIL${NC} docker_exec (should be blocked in readonly)\n"
  fi
}
check_result_exec

# docker_exec with --readonly=false
result=$(call_mcp docker docker_exec "{\"container\":\"$CONTAINER\",\"command\":\"echo hello\"}" "--readonly=false")
check_result "docker_exec (writable mode)" "$result" "Exec output"

echo ""

# ==========================================
# PROMETHEUS MCP SERVER TESTS
# ==========================================
echo -e "${YELLOW}--- Prometheus MCP Server ---${NC}"

# Wait a bit for prometheus to be ready
sleep 2

result=$(call_mcp prometheus prom_query '{"query":"up"}' "--url http://localhost:9090")
check_result "prom_query (up)" "$result" "Result type"

result=$(call_mcp prometheus prom_query_range '{"query":"up","step":"30s"}' "--url http://localhost:9090")
check_result "prom_query_range (up)" "$result" "Range Query"

result=$(call_mcp prometheus prom_alerts '{}' "--url http://localhost:9090")
check_result "prom_alerts" "$result" ""

result=$(call_mcp prometheus prom_targets '{"state":"active"}' "--url http://localhost:9090")
check_result "prom_targets" "$result" "targets"

result=$(call_mcp prometheus prom_rules '{"type":"all"}' "--url http://localhost:9090")
check_result "prom_rules" "$result" "Rules"

result=$(call_mcp prometheus prom_series '{"match":"up"}' "--url http://localhost:9090")
check_result "prom_series" "$result" "series"

result=$(call_mcp prometheus prom_label_values '{"label_name":"job"}' "--url http://localhost:9090")
check_result "prom_label_values" "$result" "values"

echo ""

# ==========================================
# FILES MCP SERVER TESTS
# ==========================================
echo -e "${YELLOW}--- Files MCP Server ---${NC}"

SAMPLE_DIR="$(cd "$(dirname "$0")/sample-files" && pwd -W 2>/dev/null || pwd)"

result=$(call_mcp files file_analyze "{\"path\":\"${SAMPLE_DIR}/Dockerfile\"}")
check_result "file_analyze (Dockerfile)" "$result" "Dockerfile Analysis"

result=$(call_mcp files file_analyze "{\"path\":\"${SAMPLE_DIR}/deploy.yaml\"}")
check_result "file_analyze (K8s YAML)" "$result" "Kubernetes YAML Analysis"

result=$(call_mcp files file_analyze "{\"path\":\"${SAMPLE_DIR}/main.tf\"}")
check_result "file_analyze (Terraform)" "$result" "Terraform Analysis"

result=$(call_mcp files file_analyze "{\"path\":\"${SAMPLE_DIR}/compose.yaml\"}")
check_result "file_analyze (Compose)" "$result" "Compose Analysis"

result=$(call_mcp files file_lint "{\"path\":\"${SAMPLE_DIR}/Dockerfile\"}")
check_result "file_lint (bad Dockerfile)" "$result" "CRITICAL"

result=$(call_mcp files file_lint "{\"path\":\"${SAMPLE_DIR}/Dockerfile.good\"}")
check_result "file_lint (good Dockerfile)" "$result" ""

result=$(call_mcp files file_lint "{\"path\":\"${SAMPLE_DIR}/deploy.yaml\"}")
check_result "file_lint (bad K8s)" "$result" "WARNING"

result=$(call_mcp files file_lint "{\"path\":\"${SAMPLE_DIR}/deploy-good.yaml\"}")
check_result "file_lint (good K8s)" "$result" ""

result=$(call_mcp files file_lint "{\"path\":\"${SAMPLE_DIR}/main.tf\"}")
check_result "file_lint (Terraform)" "$result" ""

result=$(call_mcp files file_lint "{\"path\":\"${SAMPLE_DIR}/compose.yaml\"}")
check_result "file_lint (Compose)" "$result" ""

result=$(call_mcp files file_validate "{\"path\":\"${SAMPLE_DIR}/Dockerfile\"}")
check_result "file_validate (Dockerfile)" "$result" "Validation passed"

result=$(call_mcp files file_validate "{\"path\":\"${SAMPLE_DIR}/deploy.yaml\"}")
check_result "file_validate (YAML)" "$result" "Validation passed"

result=$(call_mcp files file_validate "{\"path\":\"${SAMPLE_DIR}/main.tf\"}")
check_result "file_validate (Terraform)" "$result" "Validation passed"

result=$(call_mcp files file_scan_dir "{\"dir\":\"${SAMPLE_DIR}\"}")
check_result "file_scan_dir" "$result" "infrastructure files"

echo ""

# ==========================================
# SUMMARY
# ==========================================
echo "========================================"
printf "  Results: ${GREEN}%d passed${NC}, ${RED}%d failed${NC}, %d total\n" "$PASS" "$FAIL" "$TOTAL"
echo "========================================"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
