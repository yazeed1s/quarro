#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if ! command -v golines >/dev/null 2>&1; then
    echo "error: golines is not installed"
    echo "install with:"
    echo "  go install github.com/segmentio/golines@latest"
    exit 1
fi

if ! command -v shfmt >/dev/null 2>&1; then
    echo "error: shfmt is not installed"
    echo "install on Arch with:"
    echo "  sudo pacman -S shfmt"
    exit 1
fi

if [[ ! -x frontend/node_modules/.bin/oxfmt ]]; then
    echo "error: frontend dependencies are not installed"
    echo "run:"
    echo "  npm --prefix frontend install"
    exit 1
fi

printf '+++ formatting Go\n'

golines \
    --write-output \
    --max-len=70 \
    ./internal \
    ./main.go

printf '+++ formatting svelte/ts/js/css\n'

npm --prefix frontend run format

printf '+++ formatting shell scripts\n'

find . \
    -type f \
    \( -name '*.sh' -o -name '*.bash' \) \
    -not -path './frontend/node_modules/*' \
    -not -path './.git/*' \
    -not -path './build/*' \
    -exec shfmt -w -i 4 -ci {} +

printf 'formatting complete\n'
