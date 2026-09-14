#!/usr/bin/env bash
set -euo pipefail
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
exec bash "$script_dir/../scripts/build.sh" --client-mode cli --skip-server --skip-sidecar "$@"
