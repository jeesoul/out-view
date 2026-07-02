#!/usr/bin/env bash
# WebRTC Debug Tool
# Checks STUN reachability, NAT type detection, and connection diagnostics

set -euo pipefail

# Default STUN servers to test
STUN_SERVERS=(
    "stun.l.google.com:19302"
    "stun1.l.google.com:19302"
    "stun.qq.com:3478"
)

log() { echo "[$(date '+%H:%M:%S')] $*"; }
pass() { echo "  ✓ $*"; }
fail() { echo "  ✗ $*"; }
info() { echo "  → $*"; }

# Check if a STUN server is reachable via UDP
check_stun_reachable() {
    local server="$1"
    local host="${server%:*}"
    local port="${server##*:}"

    # Try DNS resolution first
    if ! nslookup "${host}" >/dev/null 2>&1 && ! host "${host}" >/dev/null 2>&1; then
        fail "DNS resolution failed for ${host}"
        return 1
    fi

    # Try UDP connectivity (STUN binding request)
    # Use nc if available, otherwise fall back to timeout+ping
    if command -v nc &>/dev/null; then
        if echo -n "" | nc -u -w 2 "${host}" "${port}" >/dev/null 2>&1; then
            pass "STUN ${server}: reachable"
            return 0
        fi
    fi

    # Fallback: check if port is reachable via TCP (some STUN servers support TCP)
    if command -v nc &>/dev/null; then
        if nc -z -w 2 "${host}" "${port}" >/dev/null 2>&1; then
            pass "STUN ${server}: TCP reachable (UDP may also work)"
            return 0
        fi
    fi

    # Last resort: ping the host
    if ping -c 1 -W 2 "${host}" >/dev/null 2>&1; then
        info "STUN ${server}: host reachable (UDP port ${port} not verified)"
        return 0
    fi

    fail "STUN ${server}: unreachable"
    return 1
}

# Detect NAT type using STUN
detect_nat_type() {
    log "Detecting NAT type..."

    # Get external IP via STUN (simplified detection)
    local external_ip=""

    # Try to get external IP using curl/wget
    if command -v curl &>/dev/null; then
        external_ip=$(curl -s --max-time 5 https://api.ipify.org 2>/dev/null || echo "")
    elif command -v wget &>/dev/null; then
        external_ip=$(wget -qO- --timeout=5 https://api.ipify.org 2>/dev/null || echo "")
    fi

    if [ -n "${external_ip}" ]; then
        info "External IP: ${external_ip}"

        # Get local IP
        local local_ip=""
        if command -v ip &>/dev/null; then
            local_ip=$(ip route get 8.8.8.8 2>/dev/null | grep -oP 'src \K\S+' || echo "")
        elif command -v ifconfig &>/dev/null; then
            local_ip=$(ifconfig | grep -oP 'inet \K\S+' | grep -v '127.0.0.1' | head -1 || echo "")
        fi

        if [ -n "${local_ip}" ]; then
            info "Local IP: ${local_ip}"
            if [ "${external_ip}" = "${local_ip}" ]; then
                info "NAT Type: No NAT (direct connection)"
            else
                info "NAT Type: Behind NAT (${local_ip} → ${external_ip})"
                info "Note: Full NAT type detection requires STUN RFC 5780 support"
            fi
        fi
    else
        info "Could not determine external IP (no internet access or curl/wget unavailable)"
    fi
}

# Check WebRTC sidecar process
check_sidecar() {
    log "Checking WebRTC sidecar..."

    # Check if sidecar binary exists
    local sidecar_paths=(
        "./webrtc-sidecar/sidecar"
        "./webrtc-sidecar/sidecar.exe"
        "/usr/local/bin/outview-sidecar"
        "${HOME}/.local/bin/outview-sidecar"
    )

    local found=false
    for path in "${sidecar_paths[@]}"; do
        if [ -f "${path}" ]; then
            pass "Sidecar binary found: ${path}"
            found=true
            break
        fi
    done

    if [ "${found}" = "false" ]; then
        info "Sidecar binary not found in standard locations"
        info "Build with: cd webrtc-sidecar && go build -o sidecar ./cmd/sidecar"
    fi

    # Check if sidecar is running
    if command -v pgrep &>/dev/null; then
        if pgrep -f "outview-sidecar\|webrtc-sidecar" >/dev/null 2>&1; then
            pass "Sidecar process is running"
        else
            info "Sidecar process not running"
        fi
    fi

    # Check IPC socket
    local socket_paths=(
        "/tmp/outview-webrtc.sock"
        "/tmp/webrtc-sidecar.sock"
        "\\\\.\\pipe\\outview-webrtc"
    )

    for sock in "${socket_paths[@]}"; do
        if [ -S "${sock}" ] 2>/dev/null; then
            pass "IPC socket found: ${sock}"
        fi
    done
}

# Check network interfaces
check_network_interfaces() {
    log "Checking network interfaces..."

    if command -v ip &>/dev/null; then
        local interfaces
        interfaces=$(ip link show | grep -E "^[0-9]+:" | awk '{print $2}' | tr -d ':')
        for iface in ${interfaces}; do
            local state
            state=$(ip link show "${iface}" | grep -oP 'state \K\S+' || echo "UNKNOWN")
            if [ "${state}" = "UP" ]; then
                pass "Interface ${iface}: UP"
            else
                info "Interface ${iface}: ${state}"
            fi
        done
    elif command -v ifconfig &>/dev/null; then
        ifconfig | grep -E "^[a-z]" | awk '{print $1}' | while read -r iface; do
            pass "Interface ${iface}: present"
        done
    fi
}

# Run all checks
main() {
    echo "=== outView WebRTC Debug Tool ==="
    echo "Date: $(date)"
    echo ""

    log "Checking STUN server reachability..."
    local stun_ok=0
    for server in "${STUN_SERVERS[@]}"; do
        if check_stun_reachable "${server}"; then
            ((stun_ok++))
        fi
    done

    if [ "${stun_ok}" -eq 0 ]; then
        fail "No STUN servers reachable - WebRTC may not work"
        echo ""
        echo "Troubleshooting:"
        echo "  1. Check firewall rules (UDP port 3478 must be open)"
        echo "  2. Check DNS resolution"
        echo "  3. Consider using TURN server as fallback"
    else
        pass "${stun_ok}/${#STUN_SERVERS[@]} STUN servers reachable"
    fi

    echo ""
    detect_nat_type

    echo ""
    check_network_interfaces

    echo ""
    check_sidecar

    echo ""
    echo "=== Debug complete ==="
    echo "For more help: docs/webrtc-troubleshooting.md"
}

main "$@"
