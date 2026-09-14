#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
# shellcheck source=common.sh
source "$script_dir/common.sh"
project_root=$(outview_project_root)
version=$(outview_version "$project_root")

client_mode=cli
skip_server=false
skip_client=false
skip_sidecar=false
release=false
output_directory=

usage() {
  cat <<'USAGE'
Usage: scripts/build.sh [options]
  --client-mode cli|gui|both  Select client artifacts (default: cli)
  --skip-server               Do not build the Java server
  --skip-client               Do not build the Go client
  --skip-sidecar              Do not build the experimental sidecar
  --release                   Cross-build release artifacts
  --output PATH               Output directory; it must not already exist
USAGE
}

while (($#)); do
  case "$1" in
    --client-mode) client_mode=${2:?missing client mode}; shift 2 ;;
    --skip-server) skip_server=true; shift ;;
    --skip-client) skip_client=true; shift ;;
    --skip-sidecar) skip_sidecar=true; shift ;;
    --release) release=true; shift ;;
    --output) output_directory=${2:?missing output path}; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$client_mode" in cli|gui|both) ;; *) echo "Invalid client mode: $client_mode" >&2; exit 2 ;; esac
if [[ -z "$output_directory" ]]; then
  if $release; then
    output_directory="$project_root/release/outview-$version"
  else
    output_directory="$project_root/artifacts/outview-$version"
  fi
elif [[ "$output_directory" != /* && ! "$output_directory" =~ ^[A-Za-z]:[/\\] ]]; then
  output_directory="$project_root/$output_directory"
fi

if [[ -e "$output_directory" ]]; then
  echo "Output directory already exists; refusing to overwrite it: $output_directory" >&2
  exit 1
fi
destination_parent=$(dirname "$output_directory")
mkdir -p "$destination_parent"
destination_parent=$(cd "$destination_parent" && pwd -P)
destination_leaf=$(basename "$output_directory")
case "$destination_leaf" in ''|.|..) echo 'Unsafe output directory.' >&2; exit 1 ;; esac
destination="$destination_parent/$destination_leaf"
staging="$destination_parent/.outview-staging-$$-${RANDOM:-0}"
mkdir "$staging"
published=false
cleanup() {
  if ! $published && [[ -d "$staging" ]]; then
    case "$staging" in "$destination_parent"/.outview-staging-*) ;; *) echo "Refusing unsafe staging cleanup: $staging" >&2; return 1 ;; esac
    rm -rf -- "$staging"
  fi
}
trap cleanup EXIT

if [[ -n "${SOURCE_DATE_EPOCH:-}" ]]; then
  build_date=$(date -u -d "@$SOURCE_DATE_EPOCH" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -r "$SOURCE_DATE_EPOCH" '+%Y-%m-%dT%H:%M:%SZ')
else
  build_date=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
fi

if ! $skip_server; then
  outview_resolve_java >/dev/null
  maven=$(outview_resolve_maven "$project_root")
  echo "Building Java server v$version"
  (cd "$project_root" && "$maven" clean package -DskipTests)
  test -f "$project_root/target/outview-server.jar"
  cp "$project_root/target/outview-server.jar" "$staging/outview-server.jar"
fi

go_command=
if ! $skip_client || ! $skip_sidecar; then
  go_command=$(outview_resolve_go)
fi
if $release; then
  platforms=(windows/amd64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64)
elif [[ -n "$go_command" ]]; then
  platforms=("$("$go_command" env GOOS)/$("$go_command" env GOARCH)")
else
  platforms=()
fi

if ! $skip_client; then
  for platform in "${platforms[@]}"; do
    goos=${platform%/*}
    goarch=${platform#*/}
    extension=
    [[ "$goos" == windows ]] && extension=.exe
    platform_directory="$staging/client/$goos"
    mkdir -p "$platform_directory"
    if [[ "$client_mode" == cli || "$client_mode" == both ]]; then
      output="$platform_directory/outview-client-cli-$goos-$goarch$extension"
      echo "Building CLI client for $goos/$goarch"
      (cd "$project_root/client" && GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 "$go_command" build -trimpath -ldflags "-s -w -X main.Version=$version -X main.BuildDate=$build_date" -o "$output" ./cmd/outview-client)
    fi
    if [[ "$client_mode" == gui || "$client_mode" == both ]]; then
      [[ "$goos" == windows && "$goarch" == amd64 ]] || continue
      output="$platform_directory/outview-client-gui-$goos-$goarch$extension"
      echo 'Building GUI client for windows/amd64; a working CGO C compiler is required'
      (cd "$project_root/client" && GOOS=windows GOARCH=amd64 CGO_ENABLED=1 "$go_command" build -trimpath -ldflags "-s -w -H windowsgui -X main.Version=$version -X main.BuildDate=$build_date" -o "$output" ./cmd/outview-gui)
    fi
  done
fi

if ! $skip_sidecar; then
  for platform in "${platforms[@]}"; do
    goos=${platform%/*}
    goarch=${platform#*/}
    extension=
    [[ "$goos" == windows ]] && extension=.exe
    platform_directory="$staging/webrtc-sidecar/$goos"
    mkdir -p "$platform_directory"
    output="$platform_directory/outview-sidecar-$goos-$goarch$extension"
    echo "Building sidecar for $goos/$goarch"
    (cd "$project_root/webrtc-sidecar" && GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 "$go_command" build -trimpath -ldflags '-s -w' -o "$output" ./cmd/sidecar)
  done
fi

for file in README.md USER_MANUAL.md RELEASE_SUMMARY.md CHANGELOG.md LICENSE; do
  [[ -f "$project_root/$file" ]] && cp "$project_root/$file" "$staging/"
done
[[ -f "$project_root/src/main/resources/application.yml" ]] && cp "$project_root/src/main/resources/application.yml" "$staging/application.yml.example"
mkdir "$staging/scripts"
mkdir "$staging/docs"
cp "$project_root"/docs/*.md "$staging/docs/"
cp "$script_dir/common.ps1" "$script_dir/start-server.ps1" "$script_dir/start-server.bat" \
  "$script_dir/common.sh" "$script_dir/start-server.sh" "$staging/scripts/"
case "$staging" in "$destination_parent"/.outview-staging-*) ;; *) echo "Refusing unsafe publish source: $staging" >&2; exit 1 ;; esac
case "$destination" in "$destination_parent"/*) ;; *) echo "Refusing unsafe publish destination: $destination" >&2; exit 1 ;; esac
mv "$staging" "$destination"
published=true
echo "Build completed: $destination"
