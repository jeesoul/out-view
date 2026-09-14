#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
# shellcheck source=common.sh
source "$script_dir/common.sh"
project_root=$(outview_project_root)
skip_java=false
skip_client=false
skip_sidecar=false
skip_contracts=false

while (($#)); do
  case "$1" in
    --skip-java) skip_java=true ;;
    --skip-client) skip_client=true ;;
    --skip-sidecar) skip_sidecar=true ;;
    --skip-script-contracts) skip_contracts=true ;;
    -h|--help)
      echo 'Usage: scripts/test.sh [--skip-java] [--skip-client] [--skip-sidecar] [--skip-script-contracts]'
      exit 0
      ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done

if ! $skip_java; then
  outview_resolve_java >/dev/null
  maven=$(outview_resolve_maven "$project_root")
  (cd "$project_root" && "$maven" test)
fi
if ! $skip_client; then
  go_command=$(outview_resolve_go)
  (cd "$project_root/client" && "$go_command" test -count=1 -timeout=120s -tags=headless_test ./...)
  (cd "$project_root/client" && "$go_command" test -count=1 -timeout=120s -tags=integration ./test/integration)
  (cd "$project_root/client" && "$go_command" test -count=1 -timeout=120s -tags=ci ./cmd/outview-gui)
fi
if ! $skip_sidecar; then
  go_command=$(outview_resolve_go)
  (cd "$project_root/webrtc-sidecar" && "$go_command" test -count=1 -timeout=120s ./...)
fi
if ! $skip_contracts; then
  bash "$script_dir/tests/build-contract.sh"
  if command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$script_dir/tests/build-contract.Tests.ps1"
  fi
fi
echo 'All selected test suites passed.'
