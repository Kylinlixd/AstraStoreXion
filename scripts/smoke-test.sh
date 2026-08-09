#!/usr/bin/env bash
set -euo pipefail

: "${XION_BASE_URL:?XION_BASE_URL is required}"
: "${XION_SERVICE_TOKEN:?XION_SERVICE_TOKEN is required}"

if [[ $# -ne 1 ]]; then
  echo "usage: XION_BASE_URL=... XION_SERVICE_TOKEN=... $0 <fixture>" >&2
  exit 2
fi

fixture=$1
if [[ ! -f "$fixture" ]]; then
  echo "fixture does not exist: $fixture" >&2
  exit 2
fi

for command in curl python3 file; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "required command is missing: $command" >&2
    exit 2
  fi
done

if command -v sha256sum >/dev/null 2>&1; then
  checksum() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  checksum() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  echo "sha256sum or shasum is required" >&2
  exit 2
fi

base_url=${XION_BASE_URL%/}
work_dir=$(mktemp -d)
file_id=""

cleanup() {
  if [[ -n "$file_id" ]]; then
    curl --silent --show-error --connect-timeout 3 --max-time 15 \
      -X DELETE \
      -H "Authorization: Bearer ${XION_SERVICE_TOKEN}" \
      "${base_url}/api/v1/files/${file_id}" >/dev/null 2>&1 || true
  fi
  rm -rf -- "$work_dir"
}
trap cleanup EXIT

curl --silent --show-error --fail --connect-timeout 3 --max-time 15 \
  "${base_url}/readyz" >/dev/null

content_type=$(file --brief --mime-type "$fixture")
upload_json="${work_dir}/upload.json"
curl --silent --show-error --fail-with-body --connect-timeout 3 --max-time 120 \
  -H "Authorization: Bearer ${XION_SERVICE_TOKEN}" \
  -F "file=@${fixture};type=${content_type}" \
  -F 'metadata={"source":"smoke-test"}' \
  "${base_url}/api/v1/files" >"$upload_json"

file_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["file_id"])' "$upload_json")
remote_checksum=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["checksum"])' "$upload_json")
local_checksum=$(checksum "$fixture")
if [[ "$local_checksum" != "$remote_checksum" ]]; then
  echo "upload checksum mismatch" >&2
  exit 1
fi

status_json="${work_dir}/status.json"
curl --silent --show-error --fail --connect-timeout 3 --max-time 15 \
  -H "Authorization: Bearer ${XION_SERVICE_TOKEN}" \
  "${base_url}/api/v1/files/${file_id}/status" >"$status_json"
status_checksum=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["checksum"])' "$status_json")
if [[ "$local_checksum" != "$status_checksum" ]]; then
  echo "status checksum mismatch" >&2
  exit 1
fi

download_path="${work_dir}/downloaded"
curl --silent --show-error --fail --connect-timeout 3 --max-time 120 \
  -H "Authorization: Bearer ${XION_SERVICE_TOKEN}" \
  "${base_url}/api/v1/files/${file_id}" >"$download_path"
download_checksum=$(checksum "$download_path")
if [[ "$local_checksum" != "$download_checksum" ]]; then
  echo "download checksum mismatch" >&2
  exit 1
fi

curl --silent --show-error --fail --connect-timeout 3 --max-time 15 \
  -X DELETE \
  -H "Authorization: Bearer ${XION_SERVICE_TOKEN}" \
  "${base_url}/api/v1/files/${file_id}" >/dev/null
file_id=""

echo "smoke test passed: $(basename "$fixture") sha256=${local_checksum}"
