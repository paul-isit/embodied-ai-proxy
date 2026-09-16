#!/usr/bin/env bash
set -e

# ==============================================================================
# Embodied AI Proxy - Build & Runner Script
# Launches LLM Proxy (:8081) and Backend Server (:8080) in the background,
# then launches the TUI in the foreground.
# Automatically cleans up all background processes on exit.
# ==============================================================================

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${PROJECT_ROOT}/bin"
DATA_DIR="${PROJECT_ROOT}/data"
LOG_DIR="${DATA_DIR}/logs"

# Colors for terminal output
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Process IDs
PROXY_PID=""
BACKEND_PID=""

# Help message
show_help() {
    echo -e "${CYAN}Embodied AI Proxy - Runner${NC}"
    echo ""
    echo "Usage: ./run.sh [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  (no args)        Run existing binaries (Proxy + Backend + TUI)"
    echo "  --build, -b      Build all Go binaries first, then run"
    echo "  --headless, -s   Start Proxy and Backend only (headless / evaluation mode)"
    echo "  --build-only     Compile all binaries to bin/ and exit"
    echo "  --clean          Remove compiled binaries and logs"
    echo "  --help, -h       Show this help message"
    echo ""
}

# Cleanup function invoked on EXIT, INT, TERM
cleanup() {
    set +e
    if [ -z "$BACKEND_PID" ] && [ -z "$PROXY_PID" ]; then
        return
    fi

    echo ""
    echo -e "${YELLOW}[Runner] Shutting down services...${NC}"

    if [ -n "$BACKEND_PID" ] && kill -0 "$BACKEND_PID" 2>/dev/null; then
        echo -e "${YELLOW}[Runner] Stopping Backend (PID: ${BACKEND_PID})...${NC}"
        kill "$BACKEND_PID" 2>/dev/null
    fi

    if [ -n "$PROXY_PID" ] && kill -0 "$PROXY_PID" 2>/dev/null; then
        echo -e "${YELLOW}[Runner] Stopping LLM Proxy (PID: ${PROXY_PID})...${NC}"
        kill "$PROXY_PID" 2>/dev/null
    fi

    sleep 0.5
    echo -e "${GREEN}[Runner] All services stopped.${NC}"
}

trap cleanup EXIT INT TERM

# Ensure directories exist
mkdir -p "${BIN_DIR}" "${LOG_DIR}"

# Build all Go binaries
build_binaries() {
    echo -e "${CYAN}[Build] Building Go binaries...${NC}"
    cd "${PROJECT_ROOT}"

    echo -e "  -> Building ${GREEN}bin/llm-proxy${NC}..."
    go build -o "${BIN_DIR}/llm-proxy" ./llm-proxy/cmd/server

    echo -e "  -> Building ${GREEN}bin/backend${NC}..."
    go build -o "${BIN_DIR}/backend" ./backend/cmd/server

    echo -e "  -> Building ${GREEN}bin/tui${NC}..."
    go build -o "${BIN_DIR}/tui" ./tui/cmd/tui

    echo -e "${GREEN}[Build] Build complete!${NC}"
}

# Check if a port is in use
check_port() {
    local port=$1
    if command -v lsof >/dev/null 2>&1; then
        if lsof -i :"$port" -sTCP:LISTEN -t >/dev/null 2>&1; then
            return 0
        fi
    elif command -v ss >/dev/null 2>&1; then
        if ss -ltn | grep -q ":${port} "; then
            return 0
        fi
    fi
    return 1
}

# Wait for a port to start listening
wait_for_port() {
    local port=$1
    local name=$2
    local timeout=10
    local elapsed=0

    while ! check_port "$port"; do
        sleep 0.2
        elapsed=$((elapsed + 1))
        if [ "$elapsed" -ge $((timeout * 5)) ]; then
            echo -e "${RED}[Runner] Timeout waiting for ${name} on port ${port}${NC}"
            return 1
        fi
    done
    return 0
}

# Parse CLI arguments
DO_BUILD=false
HEADLESS=false
BUILD_ONLY=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --build|-b)
            DO_BUILD=true
            shift
            ;;
        --build-only)
            BUILD_ONLY=true
            DO_BUILD=true
            shift
            ;;
        --headless|-s)
            HEADLESS=true
            shift
            ;;
        --clean)
            echo -e "${YELLOW}[Clean] Removing binaries and logs...${NC}"
            rm -rf "${BIN_DIR}"/* "${LOG_DIR}"/*
            echo -e "${GREEN}[Clean] Done.${NC}"
            exit 0
            ;;
        --help|-h)
            show_help
            exit 0
            ;;
        *)
            echo -e "${RED}[Runner] Unknown option: $1${NC}"
            show_help
            exit 1
            ;;
    esac
done

# Perform build if requested
if [ "$DO_BUILD" = true ]; then
    build_binaries
fi

if [ "$BUILD_ONLY" = true ]; then
    exit 0
fi

# Verify binaries exist
for bin in "llm-proxy" "backend" "tui"; do
    if [ ! -f "${BIN_DIR}/${bin}" ]; then
        echo -e "${RED}[Runner] Binary not found: ${BIN_DIR}/${bin}.${NC}"
        echo -e "${YELLOW}[Runner] Please run './run.sh --build' to build the project first.${NC}"
        exit 1
    fi
done

# Check for existing processes on ports 8080 and 8081
if check_port 8081; then
    echo -e "${RED}[Runner] Port 8081 is already in use! Is llm-proxy already running?${NC}"
    exit 1
fi

if check_port 8080; then
    echo -e "${RED}[Runner] Port 8080 is already in use! Is backend already running?${NC}"
    exit 1
fi

# 1. Start LLM Proxy in background
echo -e "${CYAN}[Runner] Starting LLM Proxy on :8081 (log: data/logs/proxy.log)...${NC}"
"${BIN_DIR}/llm-proxy" -dataDir "${DATA_DIR}" > /dev/null 2>&1 &
PROXY_PID=$!
wait_for_port 8081 "LLM Proxy"
echo -e "${GREEN}[Runner] LLM Proxy is UP (PID: ${PROXY_PID})${NC}"

# 2. Start Backend Server in background
echo -e "${CYAN}[Runner] Starting Backend Server on :8080 (log: data/logs/server.log)...${NC}"
"${BIN_DIR}/backend" -dataDir "${DATA_DIR}" > /dev/null 2>&1 &
BACKEND_PID=$!
wait_for_port 8080 "Backend Server"
echo -e "${GREEN}[Runner] Backend Server is UP (PID: ${BACKEND_PID})${NC}"

# 3. Launch TUI or wait in headless mode
if [ "$HEADLESS" = true ]; then
    echo -e "${GREEN}[Runner] Headless mode active. Services are running.${NC}"
    echo -e "${CYAN}Press Ctrl+C to stop services.${NC}"
    # Wait on child processes
    wait
else
    echo -e "${CYAN}[Runner] Launching TUI...${NC}"
    # Run TUI in foreground (interactive terminal)
    "${BIN_DIR}/tui" -serverURL "http://localhost:8080" -dataDir "${DATA_DIR}"
fi
