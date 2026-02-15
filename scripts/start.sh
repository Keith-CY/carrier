#!/bin/sh
# Carrier start script - starts the openclaw gateway daemon
# Optionally creates a systemd service unit on Linux
#
# Usage:
#   ./start.sh                    # Start daemon in foreground
#   ./start.sh --daemon           # Start daemon in background
#   ./start.sh --systemd          # Create and enable systemd service
#   ./start.sh --systemd --user   # Create user-level systemd service
#
# Environment Variables:
#   OPENCLAW_PORT       - HTTP port (default: 9090)
#   OPENCLAW_LOG_LEVEL  - Log level: debug|info|warn|error (default: info)
#   OPENCLAW_DATA_DIR   - Data directory (default: ~/.openclaw)
#   DRY_RUN             - If set, print actions without executing

set -e

# Parse arguments
MODE="foreground"
SYSTEMD_USER=""

while [ $# -gt 0 ]; do
    case "$1" in
        --daemon)
            MODE="daemon"
            ;;
        --systemd)
            MODE="systemd"
            ;;
        --user)
            SYSTEMD_USER="--user"
            ;;
        --help)
            sed -n '2,/^$/p' "$0" | sed 's/^# \?//'
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            echo "Run with --help for usage" >&2
            exit 1
            ;;
    esac
    shift
done

# Configuration
PORT="${OPENCLAW_PORT:-9090}"
LOG_LEVEL="${OPENCLAW_LOG_LEVEL:-info}"
DATA_DIR="${OPENCLAW_DATA_DIR:-$HOME/.openclaw}"

# Check if openclaw is installed
if ! command -v openclaw >/dev/null 2>&1; then
    echo "ERROR: openclaw not found in PATH" >&2
    echo "Install it first: ./install.sh" >&2
    exit 1
fi

# Foreground mode
if [ "$MODE" = "foreground" ]; then
    echo "Starting OpenClaw gateway on port ${PORT}..."
    if [ "${DRY_RUN:-}" ]; then
        echo "[DRY_RUN] openclaw gateway start --port=${PORT} --log-level=${LOG_LEVEL}"
    else
        exec openclaw gateway start --port="${PORT}" --log-level="${LOG_LEVEL}"
    fi
fi

# Daemon mode (background)
if [ "$MODE" = "daemon" ]; then
    echo "Starting OpenClaw gateway in background..."
    if [ "${DRY_RUN:-}" ]; then
        echo "[DRY_RUN] nohup openclaw gateway start --port=${PORT} --log-level=${LOG_LEVEL} >/dev/null 2>&1 &"
    else
        mkdir -p "$DATA_DIR"
        nohup openclaw gateway start --port="${PORT}" --log-level="${LOG_LEVEL}" \
            > "$DATA_DIR/gateway.log" 2>&1 &
        echo "Daemon started. Logs: $DATA_DIR/gateway.log"
        echo "PID: $!"
    fi
    exit 0
fi

# Systemd service creation
if [ "$MODE" = "systemd" ]; then
    if [ "$(uname -s)" != "Linux" ]; then
        echo "ERROR: systemd is only available on Linux" >&2
        exit 1
    fi

    if [ "$SYSTEMD_USER" = "--user" ]; then
        SERVICE_DIR="$HOME/.config/systemd/user"
        SYSTEMCTL_CMD="systemctl --user"
        WANTED_BY_TARGET="default.target"
    else
        SERVICE_DIR="/etc/systemd/system"
        SYSTEMCTL_CMD="systemctl"
        WANTED_BY_TARGET="multi-user.target"
        # Check for sudo if system-level
        if [ ! -w "$SERVICE_DIR" ] && [ -z "${DRY_RUN:-}" ]; then
            if ! command -v sudo >/dev/null 2>&1; then
                echo "ERROR: sudo required for system-level service" >&2
                exit 1
            fi
        fi
    fi

    SERVICE_FILE="${SERVICE_DIR}/openclaw-gateway.service"
    OPENCLAW_BIN="$(command -v openclaw)"
    
    # User for system service
    if [ "$SYSTEMD_USER" != "--user" ]; then
        SERVICE_USER="${USER:-openclaw}"
    fi

    echo "Creating systemd service: $SERVICE_FILE"
    echo "Using systemd install target: $WANTED_BY_TARGET"

    # Generate service unit
    SERVICE_CONTENT="[Unit]
Description=OpenClaw Gateway Daemon
After=network.target
Documentation=https://github.com/Keith-CY/carrier

[Service]
Type=simple
ExecStart=${OPENCLAW_BIN} gateway start --port=${PORT} --log-level=${LOG_LEVEL}
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal"

    if [ "$SYSTEMD_USER" != "--user" ]; then
        SERVICE_CONTENT="${SERVICE_CONTENT}
User=${SERVICE_USER}
Group=${SERVICE_USER}"
    fi

    SERVICE_CONTENT="${SERVICE_CONTENT}

# Security hardening
PrivateTmp=true
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=${DATA_DIR}

# Environment
Environment=OPENCLAW_PORT=${PORT}
Environment=OPENCLAW_LOG_LEVEL=${LOG_LEVEL}
Environment=OPENCLAW_DATA_DIR=${DATA_DIR}

[Install]
WantedBy=${WANTED_BY_TARGET}"

    if [ "${DRY_RUN:-}" ]; then
        echo "[DRY_RUN] Would write to: $SERVICE_FILE"
        echo "---"
        echo "$SERVICE_CONTENT"
        echo "---"
        echo "[DRY_RUN] ${SYSTEMCTL_CMD} daemon-reload"
        echo "[DRY_RUN] ${SYSTEMCTL_CMD} enable openclaw-gateway.service"
        echo "[DRY_RUN] ${SYSTEMCTL_CMD} start openclaw-gateway.service"
    else
        # Write service file
        if [ "$SYSTEMD_USER" = "--user" ]; then
            mkdir -p "$SERVICE_DIR"
            printf "%s\n" "$SERVICE_CONTENT" > "$SERVICE_FILE"
        else
            printf "%s\n" "$SERVICE_CONTENT" | sudo tee "$SERVICE_FILE" > /dev/null
        fi

        # Reload systemd
        $SYSTEMCTL_CMD daemon-reload

        echo "Service created: openclaw-gateway.service"
        echo ""
        echo "To enable and start:"
        echo "  ${SYSTEMCTL_CMD} enable openclaw-gateway.service"
        echo "  ${SYSTEMCTL_CMD} start openclaw-gateway.service"
        echo ""
        echo "To check status:"
        echo "  ${SYSTEMCTL_CMD} status openclaw-gateway.service"
    fi
fi
