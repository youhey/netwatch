#!/usr/bin/env bash
set -euo pipefail

BIN_PATH="${1:-dist/netwatchd-linux-armv6}"
TOOL_PATH="${2:-$(dirname "${BIN_PATH}")/netwatch-jsonl-linux-armv6}"

if [[ ! -f "${BIN_PATH}" ]]; then
  echo "binary not found: ${BIN_PATH}" >&2
  exit 1
fi

if ! id netwatch >/dev/null 2>&1; then
  sudo useradd --system --home-dir /var/lib/netwatch --shell /usr/sbin/nologin netwatch
fi

sudo install -d -o root -g root /etc/netwatch
sudo install -d -o netwatch -g netwatch /var/lib/netwatch
sudo install -m 0755 "${BIN_PATH}" /usr/local/bin/netwatchd
if [[ -f "${TOOL_PATH}" ]]; then
  sudo install -m 0755 "${TOOL_PATH}" /usr/local/bin/netwatch-jsonl
fi
sudo install -m 0644 configs/netwatch.example.json /etc/netwatch/netwatch.json
sudo install -m 0644 deploy/systemd/netwatch.service /etc/systemd/system/netwatch.service
sudo systemctl daemon-reload

echo "installed netwatchd. Review /etc/netwatch/netwatch.json, then run: sudo systemctl enable --now netwatch"
