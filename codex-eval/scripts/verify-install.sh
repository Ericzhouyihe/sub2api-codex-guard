#!/bin/sh
# Preflight check for a Sub2API plugins installer. Does not modify the server.
set -eu

installer="${1:-}"
if [ -z "$installer" ]; then
  echo "usage: $0 <path-to-sub2api-plugins-installer-vX.Y.Z-linux-amd64>" >&2
  exit 1
fi
if [ ! -f "$installer" ]; then
  echo "missing installer: $installer" >&2
  exit 1
fi
if [ ! -s "$installer" ]; then
  echo "installer is empty: $installer" >&2
  exit 1
fi
if [ ! -x "$installer" ]; then
  echo "warning: installer is not executable; run: chmod +x \"$installer\"" >&2
fi

echo "Installer: $installer"
echo "Size: $(wc -c < "$installer") bytes"
echo "SHA256: $(sha256sum "$installer" | awk '{print $1}')"

if [ -f "$(dirname "$installer")/SHA256SUMS" ]; then
  (cd "$(dirname "$installer")" && sha256sum -c SHA256SUMS)
fi

if command -v file >/dev/null 2>&1; then
  file "$installer"
fi
if command -v readelf >/dev/null 2>&1; then
  readelf -h "$installer" | sed -n '1,12p'
fi

cat <<'EOF'

Before running the installer:
  1. Confirm the target is the intended Sub2API v0.2.21 instance.
  2. Stop or pause traffic during the maintenance window.
  3. Confirm the installer will show a backup path before replacing /app/sub2api.
  4. Keep the backup path and this installer's SHA256 in the change record.
  5. If the health check fails, the installer must restore the original binary and restart.
EOF
