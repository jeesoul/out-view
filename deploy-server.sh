#!/usr/bin/env bash
set -euo pipefail

project_root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
# shellcheck source=scripts/common.sh
source "$project_root/scripts/common.sh"
version=$(outview_version "$project_root")

if (($# < 1 || $# > 3)); then
  echo "Usage: $0 SSH_TARGET [DEPLOY_DIR] [PACKAGE_DIR]" >&2
  echo "Example: $0 deploy@example.com /opt/outview release/outview-$version" >&2
  exit 2
fi
ssh_target=$1
deploy_dir=${2:-/opt/outview}
package_dir=${3:-$project_root/release/outview-$version}
[[ "$package_dir" == /* ]] || package_dir="$project_root/$package_dir"
case "$deploy_dir" in /*) ;; *) echo 'DEPLOY_DIR must be an absolute Unix path.' >&2; exit 2 ;; esac
case "$deploy_dir" in *[!A-Za-z0-9_./-]*) echo 'DEPLOY_DIR contains unsupported characters.' >&2; exit 2 ;; esac

jar="$package_dir/outview-server.jar"
config_example="$package_dir/application.yml.example"
test -f "$jar" || { echo "Missing release JAR: $jar" >&2; exit 1; }
test -f "$config_example" || { echo "Missing configuration example: $config_example" >&2; exit 1; }

temporary_unit=$(mktemp "${TMPDIR:-/tmp}/outview.service.XXXXXX")
trap 'rm -f -- "$temporary_unit"' EXIT
cat > "$temporary_unit" <<UNIT
[Unit]
Description=outView Remote Desktop Server
After=network.target

[Service]
Type=simple
WorkingDirectory=$deploy_dir
Environment=OUTVIEW_DATA_DIR=$deploy_dir/data
ExecStart=/usr/bin/java -jar $deploy_dir/current/outview-server.jar --spring.config.additional-location=optional:file:$deploy_dir/shared/application.yml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
UNIT

remote_release="$deploy_dir/releases/$version"
ssh "$ssh_target" "set -eu; mkdir -p '$remote_release' '$deploy_dir/shared' '$deploy_dir/data'"
scp "$jar" "$ssh_target:$remote_release/outview-server.jar"
scp "$config_example" "$ssh_target:$remote_release/application.yml.example"
scp "$temporary_unit" "$ssh_target:/tmp/outview.service.$version"
ssh "$ssh_target" "set -eu; if [ ! -e '$deploy_dir/shared/application.yml' ]; then if [ -f '$deploy_dir/application.yml' ]; then cp '$deploy_dir/application.yml' '$deploy_dir/shared/application.yml'; else cp '$remote_release/application.yml.example' '$deploy_dir/shared/application.yml'; fi; fi; ln -sfn '$remote_release' '$deploy_dir/current'; sudo install -m 0644 '/tmp/outview.service.$version' /etc/systemd/system/outview.service; rm -f '/tmp/outview.service.$version'; sudo systemctl daemon-reload; sudo systemctl enable outview; sudo systemctl restart outview; sudo systemctl status --no-pager outview"

echo "Deployed outView $version to $ssh_target:$remote_release"
echo "Existing $deploy_dir/shared/application.yml was preserved."
