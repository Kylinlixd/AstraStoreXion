#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "请使用 root 或 sudo 运行此脚本。" >&2
  exit 1
fi

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
binary_path="${XIONCTL_BINARY:-${script_dir}/../bin/xionctl}"
if [[ ! -f "${binary_path}" ]]; then
  echo "找不到 ${binary_path}，请先运行 make xionctl。" >&2
  exit 1
fi

install -o root -g root -m 0755 "${binary_path}" /usr/local/bin/xionctl

if [[ -e /etc/astrastore-xion.env ]]; then
  mode=$(stat -c '%a' /etc/astrastore-xion.env 2>/dev/null || stat -f '%Lp' /etc/astrastore-xion.env)
  if [[ "${mode}" != "600" ]]; then
    chmod 0600 /etc/astrastore-xion.env
    echo "已将 /etc/astrastore-xion.env 权限收紧为 0600。"
  fi
fi

echo "已安装 /usr/local/bin/xionctl。"
echo "示例：xionctl health；xionctl list；xionctl info <file-id>。"
