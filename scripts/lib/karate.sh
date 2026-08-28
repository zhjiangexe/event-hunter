#!/usr/bin/env bash

# Shared Karate standalone runner. Source this file from scripts in the project;
# it intentionally does not enable shell options for the caller.
EVENT_HUNTER_KARATE_JAR="${KARATE_JAR:-/tmp/event-hunter-karate-2.1.2.jar}"

event_hunter_require_karate() {
  if [[ ! -f "${EVENT_HUNTER_KARATE_JAR}" ]]; then
    echo "找不到 Karate standalone JAR：${EVENT_HUNTER_KARATE_JAR}" >&2
    echo "請設定 KARATE_JAR=/absolute/path/to/karate-2.1.2.jar" >&2
    return 2
  fi
}

event_hunter_prepare_karate_output() {
  local output_dir="$1"
  if [[ -z "${output_dir}" || "${output_dir}" == "/" || "${output_dir}" == "." ]]; then
    echo "拒絕不安全的 Karate output directory：${output_dir}" >&2
    return 2
  fi
  rm -rf -- "${output_dir}"
  mkdir -p "${output_dir}"
}

event_hunter_run_karate() {
  local output_dir="$1"
  shift
  event_hunter_require_karate
  event_hunter_prepare_karate_output "${output_dir}"
  java -jar "${EVENT_HUNTER_KARATE_JAR}" run \
    --no-pom --configdir=e2e --output="${output_dir}" "$@"
}
