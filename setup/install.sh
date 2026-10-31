#!/usr/bin/env bash
# Sets up embodied-ai: builds the proxy, then runs the ROS2-middleware setup.
# Usage: curl -fsSL https://raw.githubusercontent.com/paul-isit/embodied-ai-proxy/main/setup/install.sh | bash
#    or: ./setup/install.sh [--yes] [--prefix DIR] [--skip-middleware] [--no-shell-config]
# Puts the proxy in <prefix>/embodied-ai-proxy and the ROS workspace in <prefix>/ros2_kortex_ws (default prefix ~/workspace).
# Bash 3.2 compatible, because --skip-middleware also runs on macOS.
set -Eeuo pipefail

PROXY_REPO_URL="https://github.com/paul-isit/embodied-ai-proxy.git"
PROXY_BRANCH="${PROXY_BRANCH:-main}"
MIDDLEWARE_BRANCH="${MIDDLEWARE_BRANCH:-main}"
MIDDLEWARE_SETUP_URL="https://raw.githubusercontent.com/Jeremy-Allan/ROS2-middleware/${MIDDLEWARE_BRANCH}/setup/install.sh"
GO_BLOCK_START="# >>> embodied-ai go >>>"
GO_BLOCK_END="# <<< embodied-ai go <<<"

assume_yes=false
prefix="$HOME/workspace"
skip_middleware=false
write_shell_config=true
current_step="starting"

die() { echo "ERROR: $*" >&2; exit 1; }
step() { current_step="$1"; echo; echo "==> $1"; }

on_error() {
    echo
    echo "Setup failed during: $current_step"
    echo "Full log: $prefix/embodied-ai-setup.log"
    echo "Fix the problem and run the same command again. It continues from where it stopped."
}

parse_arguments() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --yes) assume_yes=true ;;
            --prefix) [ $# -ge 2 ] || die "--prefix needs a folder"; prefix="$2"; shift ;;
            --skip-middleware) skip_middleware=true ;;
            --no-shell-config) write_shell_config=false ;;
            -h|--help) sed -n '2,5p' "${BASH_SOURCE[0]:-/dev/null}" 2>/dev/null; exit 0 ;;
            *) die "unknown option: $1" ;;
        esac
        shift
    done
    case "$prefix" in
        /*) ;;
        *) prefix="$PWD/$prefix" ;;
    esac
}

check_environment() {
    [ "$(id -u)" -ne 0 ] || die "run this as your normal user, not root. It asks for sudo when needed."
    case "$prefix" in
        /mnt/*) die "install under your Linux home folder, not $prefix. Builds under /mnt are slow and break permissions." ;;
    esac
    if $skip_middleware; then
        case "$(uname -s)" in
            Linux|Darwin) ;;
            *) die "proxy-only setup supports Linux and macOS. On Windows, use WSL2." ;;
        esac
    else
        local os_id="" os_version=""
        if [ -r /etc/os-release ]; then
            os_id="$(. /etc/os-release && echo "$ID")"
            os_version="$(. /etc/os-release && echo "$VERSION_ID")"
        fi
        if [ "$os_id" != "ubuntu" ] || [ "$os_version" != "22.04" ]; then
            die "the full setup needs Ubuntu 22.04 (found ${os_id:-unknown} ${os_version:-}). Use WSL2, a UTM VM, Distrobox or native Ubuntu 22.04, or pass --skip-middleware for the proxy only."
        fi
    fi
    command -v sudo > /dev/null || die "sudo is required"
}

confirm_plan() {
    cat <<EOF

This setup will:
  Proxy:
    - install git and curl if missing (sudo, Ubuntu only)
    - clone embodied-ai-proxy ($PROXY_BRANCH) into $prefix/embodied-ai-proxy
    - install Go to /usr/local/go if it is missing or older than the proxy needs (sudo)
    - build the proxy binaries
EOF
    if $skip_middleware; then
        if $write_shell_config; then
            echo "    - add a marked block to $(shell_rc_file) that puts Go on your PATH"
        fi
    else
        cat <<EOF
  Middleware:
    - add the ROS 2 apt source and install ROS 2 Humble desktop, the ROS dev tools and ros2controlcli (sudo)
    - initialise rosdep (sudo)
    - clone ROS2-middleware ($MIDDLEWARE_BRANCH) and ros2_kortex (pinned commit) into $prefix/ros2_kortex_ws/src
    - install their dependencies with rosdep (sudo) and build the workspace
EOF
        if $write_shell_config; then
            echo "    - add a marked block to $(shell_rc_file) that sources $prefix/ros2_kortex_ws/activate"
        fi
    fi
    echo
    $assume_yes && return
    { : < /dev/tty; } 2> /dev/null || die "no terminal to ask for confirmation. Re-run with --yes."
    local answer
    read -r -p "Continue? [y/N] " answer < /dev/tty
    case "$answer" in
        y|Y|yes|YES) ;;
        *) echo "Cancelled. Nothing was changed."; exit 0 ;;
    esac
}

start_log() {
    mkdir -p "$prefix"
    exec > >(tee -a "$prefix/embodied-ai-setup.log") 2>&1
    echo "--- setup started $(date) ---"
    trap on_error ERR
    sudo -v
}

install_prerequisites() {
    step "Checking git and curl"
    if command -v git > /dev/null && command -v curl > /dev/null; then
        echo "Already installed."
    elif [ "$(uname -s)" = "Linux" ] && command -v apt-get > /dev/null; then
        sudo apt-get update
        sudo apt-get install -y git curl
    else
        die "install git and curl first (on macOS: xcode-select --install)"
    fi
}

fetch_proxy() {
    step "Fetching the proxy"
    local proxy_dir="$prefix/embodied-ai-proxy"
    if [ ! -d "$proxy_dir/.git" ]; then
        git clone --branch "$PROXY_BRANCH" "$PROXY_REPO_URL" "$proxy_dir"
        return
    fi
    local current_branch
    current_branch="$(git -C "$proxy_dir" rev-parse --abbrev-ref HEAD)"
    if [ "$current_branch" != "$PROXY_BRANCH" ]; then
        echo "Skipped updating the proxy: it is on '$current_branch', not '$PROXY_BRANCH'."
    elif [ -n "$(git -C "$proxy_dir" status --porcelain)" ]; then
        echo "Skipped updating the proxy: it has uncommitted changes."
    else
        git -C "$proxy_dir" pull --ff-only || echo "Skipped updating the proxy: it cannot fast-forward."
    fi
}

# True when version $1 is at least version $2, comparing dot-separated numbers.
version_at_least() {
    local have="$1" need="$2" have_part need_part
    while [ -n "$need" ]; do
        have_part="${have%%.*}"; need_part="${need%%.*}"
        [ "${have_part:-0}" -gt "$need_part" ] && return 0
        [ "${have_part:-0}" -lt "$need_part" ] && return 1
        [ "$have" = "${have#*.}" ] && have="" || have="${have#*.}"
        [ "$need" = "${need#*.}" ] && need="" || need="${need#*.}"
    done
    return 0
}

install_go() {
    step "Checking Go"
    local required_version installed_version=""
    required_version="$(awk '$1 == "go" { print $2; exit }' "$prefix/embodied-ai-proxy/go.work")"
    [ -n "$required_version" ] || die "could not read the Go version from go.work"
    export PATH="$PATH:/usr/local/go/bin"
    if command -v go > /dev/null; then
        installed_version="$(go env GOVERSION | sed 's/^go//')"
    fi
    if [ -n "$installed_version" ] && version_at_least "$installed_version" "$required_version"; then
        echo "Go $installed_version is new enough (needs $required_version)."
        return
    fi
    if [ -n "$installed_version" ] && [ "$(command -v go)" != "/usr/local/go/bin/go" ]; then
        die "Go $installed_version at $(command -v go) is older than $required_version. Remove or upgrade it, then re-run."
    fi

    local go_os go_arch
    case "$(uname -s)" in
        Linux) go_os=linux ;;
        Darwin) go_os=darwin ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64) go_arch=amd64 ;;
        aarch64|arm64) go_arch=arm64 ;;
        *) die "unsupported CPU: $(uname -m)" ;;
    esac
    echo "Installing Go $required_version to /usr/local/go"
    local archive="/tmp/go${required_version}.${go_os}-${go_arch}.tar.gz"
    curl -fsSL -o "$archive" "https://go.dev/dl/go${required_version}.${go_os}-${go_arch}.tar.gz"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "$archive"
    rm -f "$archive"
}

build_proxy() {
    step "Building the proxy"
    cd "$prefix/embodied-ai-proxy"
    ./run.sh --build-only
}

run_middleware_setup() {
    step "Running the ROS2-middleware setup"
    local middleware_arguments=(--yes --prefix "$prefix")
    $write_shell_config || middleware_arguments+=(--no-shell-config)
    curl -fsSL "$MIDDLEWARE_SETUP_URL" | MIDDLEWARE_BRANCH="$MIDDLEWARE_BRANCH" bash -s -- "${middleware_arguments[@]}"
}

shell_rc_file() {
    case "$(basename "${SHELL:-bash}")" in
        zsh) echo "$HOME/.zshrc" ;;
        bash) echo "$HOME/.bashrc" ;;
        *) echo "" ;;
    esac
}

# Only for --skip-middleware. The full setup's activate script already adds Go to PATH.
write_go_path_block() {
    $write_shell_config || return 0
    [ -d /usr/local/go/bin ] || return 0
    step "Adding Go to your PATH"
    local rc_file
    rc_file="$(shell_rc_file)"
    local path_line='case ":$PATH:" in *:/usr/local/go/bin:*) ;; *) export PATH="$PATH:/usr/local/go/bin" ;; esac'
    if [ -z "$rc_file" ]; then
        echo "Your shell is not bash or zsh. Add /usr/local/go/bin to your PATH yourself."
    elif grep -qsF "$GO_BLOCK_START" "$rc_file"; then
        echo "Go block already in $rc_file."
    else
        printf '\n%s\n%s\n%s\n' "$GO_BLOCK_START" "$path_line" "$GO_BLOCK_END" >> "$rc_file"
        echo "Added a block to $rc_file between the '$GO_BLOCK_START' and '$GO_BLOCK_END' lines."
    fi
}

check_proxy() {
    step "Checking the proxy build"
    local binary_count
    binary_count="$(find "$prefix/embodied-ai-proxy/bin" -maxdepth 1 -type f -perm -u+x | wc -l | tr -d ' ')"
    [ "$binary_count" -ge 3 ] || die "expected 3 proxy binaries in $prefix/embodied-ai-proxy/bin, found $binary_count"
    echo "OK  $binary_count proxy binaries built"
}

print_next_steps() {
    local config_file="$prefix/embodied-ai-proxy/data/config/config.json"
    echo
    echo "Proxy ready."
    echo
    if $skip_middleware; then
        cat <<EOF
This device has no middleware. Point the proxy at it before starting:
  Edit "rosbridge_url" in $config_file
  to ws://<middleware address>:9090
  The middleware's rosbridge must be reachable from this device.

EOF
    fi
    cat <<EOF
Before you start, check the proxy config: $config_file
  - LLM provider, model and API key (default: local Ollama, gemma3:1b)
  - rosbridge_url (default: ws://localhost:9090)

Start the proxy and terminal UI:
  cd $prefix/embodied-ai-proxy && ./run.sh

Local LLM: install Ollama (https://ollama.com), then run: ollama pull gemma3:1b
Inside a VM, models run on CPU, so small models are the practical choice.
EOF
}

main() {
    parse_arguments "$@"
    check_environment
    confirm_plan
    start_log
    install_prerequisites
    fetch_proxy
    install_go
    build_proxy
    check_proxy
    if $skip_middleware; then
        write_go_path_block
    else
        run_middleware_setup
    fi
    print_next_steps
}

# Called on the last line so a partly downloaded script runs nothing.
main "$@"
