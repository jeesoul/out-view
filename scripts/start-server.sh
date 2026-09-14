#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
# shellcheck source=common.sh
source "$script_dir/common.sh"
project_root=$(outview_project_root)
jar_path=
data_dir=
java_command=
server_arguments=()

while (($#)); do
  case "$1" in
    --jar) jar_path=${2:?missing JAR path}; shift 2 ;;
    --data-dir) data_dir=${2:?missing data directory}; shift 2 ;;
    --java) java_command=${2:?missing Java command}; shift 2 ;;
    --) shift; server_arguments=("$@"); break ;;
    -h|--help)
      echo 'Usage: scripts/start-server.sh [--jar PATH] [--data-dir PATH] [--java COMMAND] [-- SERVER_ARGS...]'
      exit 0
      ;;
    *) server_arguments+=("$1"); shift ;;
  esac
done

if [[ -z "$jar_path" ]]; then
  if [[ -f "$project_root/outview-server.jar" ]]; then
    jar_path="$project_root/outview-server.jar"
  else
    jar_path="$project_root/target/outview-server.jar"
  fi
fi
[[ "$jar_path" == /* || "$jar_path" =~ ^[A-Za-z]:[/\\] ]] || jar_path="$project_root/$jar_path"
if [[ ! -f "$jar_path" ]]; then
  echo "Server JAR not found: $jar_path. Run scripts/build.sh first." >&2
  exit 1
fi
jar_path=$(cd "$(dirname "$jar_path")" && pwd -P)/$(basename "$jar_path")

data_dir=${data_dir:-${OUTVIEW_DATA_DIR:-$project_root/data}}
[[ "$data_dir" == /* || "$data_dir" =~ ^[A-Za-z]:[/\\] ]] || data_dir="$project_root/$data_dir"
mkdir -p "$data_dir"
data_dir=$(cd "$data_dir" && pwd -P)
export OUTVIEW_DATA_DIR="$data_dir"
java_command=${java_command:-$(outview_resolve_java)}

echo "Starting outView server in $project_root"
echo "Persistent data directory: $OUTVIEW_DATA_DIR"
cd "$project_root"
exec "$java_command" -jar "$jar_path" "${server_arguments[@]}"
