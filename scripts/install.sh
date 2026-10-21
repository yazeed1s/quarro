#!/bin/bash
set -euo pipefail

exit_with_error() {
    echo "Error: $1" >&2
    exit 1
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || exit_with_error "\"$1\" is required but not installed"
}

require_cmd curl
require_cmd unzip

# get the system's architecture for linux and macOS
get_architecture_unix() {
    case "$(uname -m)" in
        x86_64) echo "amd-64-bit" ;;
        arm64 | aarch64) echo "arm-64-bit" ;;
        *) exit_with_error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# get the OS and architecture
case "$(uname -s)" in
    Darwin)
        os="mac"
        arch=$(get_architecture_unix)
        ;;
    Linux)
        os="linux"
        arch=$(get_architecture_unix)
        ;;
    *) exit_with_error "Unsupported OS: $(uname -s)" ;;
esac

github_repo="yazeed1s/quarro"
bin_dir="/usr/local/bin"
executable="quarro"

# resolve the latest release tag unless one is pinned via QUARRO_INSTALL_VERSION
version="${QUARRO_INSTALL_VERSION:-}"
if [ -z "${version}" ]; then
    echo "Looking up the latest release..."
    version=$(curl -fsSL "https://api.github.com/repos/${github_repo}/releases/latest" |
        grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
    [ -n "${version}" ] || exit_with_error "Could not determine the latest release version"
fi

url="https://github.com/${github_repo}/releases/download/${version}/${os}-${arch}.zip"

work_dir=$(mktemp -d)
trap 'rm -rf "${work_dir}"' EXIT

archive="${work_dir}/${os}-${arch}.zip"
echo "Downloading binary from ${url}..."
curl -L --show-error --fail -o "${archive}" "${url}" || exit_with_error "Failed to download binary"

unzip -q "${archive}" -d "${work_dir}/extracted" || exit_with_error "Failed to unzip binary"

binary_path="${work_dir}/extracted/${os}-${arch}/${executable}"
[ -f "${binary_path}" ] || exit_with_error "Expected binary not found in archive: ${os}-${arch}/${executable}"
chmod +x "${binary_path}"

echo "Installing to ${bin_dir}..."
if [ -w "${bin_dir}" ]; then
    mv "${binary_path}" "${bin_dir}/${executable}" || exit_with_error "Failed to move binary"
else
    sudo mv "${binary_path}" "${bin_dir}/${executable}" || exit_with_error "Failed to move binary (needs sudo for ${bin_dir})"
fi

echo "${executable} ${version} installed successfully to ${bin_dir}/${executable}"
