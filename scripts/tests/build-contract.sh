#!/usr/bin/env bash
set -euo pipefail

project_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
test_parent=$(cd "${TMPDIR:-/tmp}" && pwd -P)
temp_root=$(mktemp -d "$test_parent/outview-script-tests.XXXXXX")
cleanup() {
  case "$temp_root" in "$test_parent"/outview-script-tests.*) rm -rf -- "$temp_root" ;; *) echo "Unsafe test cleanup path" >&2; return 1 ;; esac
}
trap cleanup EXIT

cli_output="$temp_root/cli-build"
bash "$project_root/scripts/build.sh" --client-mode cli --skip-server --skip-sidecar --output "$cli_output"
case "$(go env GOOS)" in
  windows) extension=.exe ;;
  *) extension= ;;
esac
cli_binary="$cli_output/client/$(go env GOOS)/outview-client-cli-$(go env GOOS)-$(go env GOARCH)$extension"
test -f "$cli_binary"
"$cli_binary" -version | grep -Eq 'v1\.2\.1\b'

protected_output="$temp_root/existing-release"
mkdir -p "$protected_output"
printf '%s\n' 'user-owned=true' > "$protected_output/config.txt"
if bash "$project_root/scripts/build.sh" --client-mode cli --skip-server --skip-sidecar --output "$protected_output"; then
  echo 'build.sh overwrote an existing destination' >&2
  exit 1
fi
grep -Fxq 'user-owned=true' "$protected_output/config.txt"

fake_java="$temp_root/fake-java"
cat > "$fake_java" <<'FAKEJAVA'
#!/usr/bin/env bash
printf '%s|%s|%s\n' "$PWD" "$OUTVIEW_DATA_DIR" "$*" > "$OUTVIEW_TEST_CAPTURE"
FAKEJAVA
chmod +x "$fake_java"
fake_jar="$temp_root/outview-server.jar"
printf '%s\n' fixture > "$fake_jar"
expected_jar=$(cd "$(dirname "$fake_jar")" && pwd -P)/$(basename "$fake_jar")
export OUTVIEW_TEST_CAPTURE="$temp_root/start-capture.txt"
data_dir="$temp_root/persistent-data"
bash "$project_root/scripts/start-server.sh" --java "$fake_java" --jar "$fake_jar" --data-dir "$data_dir"
expected_data=$(cd "$data_dir" && pwd -P)
grep -Fqx "$project_root|$expected_data|-jar $expected_jar" "$OUTVIEW_TEST_CAPTURE"

echo 'Shell build script contract tests passed.'
