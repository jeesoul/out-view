#!/usr/bin/env bash

outview_project_root() {
  cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P
}

outview_version() {
  local project_root=${1:-$(outview_project_root)}
  awk '
    /<artifactId>outview-server<\/artifactId>/ { project_artifact = 1; next }
    project_artifact && match($0, /<version>[^<]+/) {
      value = substr($0, RSTART + 9, RLENGTH - 9)
      print value
      exit
    }
  ' "$project_root/pom.xml"
}

outview_resolve_java() {
  if [[ -n "${JAVA_HOME:-}" && -x "$JAVA_HOME/bin/java" ]]; then
    printf '%s\n' "$JAVA_HOME/bin/java"
  elif command -v java >/dev/null 2>&1; then
    command -v java
  else
    echo 'Java was not found. Set JAVA_HOME or add java to PATH.' >&2
    return 1
  fi
}

outview_resolve_go() {
  if [[ -n "${GOROOT:-}" && -x "$GOROOT/bin/go" ]]; then
    printf '%s\n' "$GOROOT/bin/go"
  elif command -v go >/dev/null 2>&1; then
    command -v go
  else
    echo 'Go was not found. Set GOROOT or add go to PATH.' >&2
    return 1
  fi
}

outview_resolve_maven() {
  local project_root=${1:-$(outview_project_root)}
  local candidate
  for candidate in \
    "$project_root/mvnw" \
    "${MAVEN_HOME:-}/bin/mvn" \
    "${M2_HOME:-}/bin/mvn"; do
    if [[ "$candidate" != /bin/mvn && -x "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  if command -v mvn >/dev/null 2>&1; then
    command -v mvn
    return 0
  fi
  if [[ -n "${USERPROFILE:-}" ]]; then
    candidate=$(find "$USERPROFILE/.m2/wrapper/dists" -type f \( -name mvn -o -name mvn.cmd \) 2>/dev/null | sort -r | head -n 1 || true)
  else
    candidate=$(find "${HOME:-/nonexistent}/.m2/wrapper/dists" -type f \( -name mvn -o -name mvn.cmd \) 2>/dev/null | sort -r | head -n 1 || true)
  fi
  if [[ -n "$candidate" ]]; then
    printf '%s\n' "$candidate"
    return 0
  fi
  echo 'Maven was not found. Set MAVEN_HOME or M2_HOME, add mvn to PATH, or populate the standard Maven wrapper cache.' >&2
  return 1
}
