#!/usr/bin/env bash
# Optional, explicit local runtime setup. Never modifies global agent settings.
set -euo pipefail
runtime_dir=${1:?Usage: setup-hyperframes.sh /absolute/app-owned/runtime-directory}
[[ "$runtime_dir" = /* ]] || { echo 'Runtime directory must be absolute' >&2; exit 1; }
node -e 'if(Number(process.versions.node.split(".")[0])<22){console.error("Node 22+ is required");process.exit(1)}'
command -v ffmpeg >/dev/null || { echo 'Install local FFmpeg first' >&2; exit 1; }
command -v ffprobe >/dev/null || { echo 'Install local FFprobe first' >&2; exit 1; }
mkdir -p "$runtime_dir"
npm install --prefix "$runtime_dir" --save-exact --no-audit --no-fund hyperframes@0.9.33
printf 'Set VIDEOFLOW_HYPERFRAMES_BIN=%s/node_modules/.bin/hyperframes\n' "$runtime_dir"
printf 'Set HYPERFRAMES_BROWSER_PATH to your installed Chrome executable.\n'
