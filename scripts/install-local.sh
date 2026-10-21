#!/usr/bin/env bash
# build the desktop app from this checkout and install it for this user.
set -euo pipefail

usage() {
    echo "Usage: $0 [--preview|--prod]"
}

if [ "$#" -gt 1 ]; then
    usage >&2
    exit 2
fi

case "${1:---preview}" in
--preview | --dev) stage=preview ;;
--prod) stage=prod ;;
--help | -h)
    usage
    exit 0
    ;;
*)
    usage >&2
    exit 2
    ;;
esac

cd "$(dirname "$0")/.."

if ! command -v wails3 >/dev/null 2>&1; then
    echo 'wails3 is required; see the prerequisites in README.md' >&2
    exit 1
fi

if [ "$stage" = preview ]; then
    wails3 build DEV=true
else
    wails3 build
fi

if [ "$(cat frontend/dist/app-stage.json)" != "\"$stage\"" ]; then
    echo "Build did not produce a $stage frontend" >&2
    exit 1
fi

binary=bin/quarro
if [ ! -x "$binary" ]; then
    echo "Build did not produce $binary" >&2
    exit 1
fi

bin_dir="$HOME/.local/bin"
mkdir -p "$bin_dir"
staged=$(mktemp "$bin_dir/.quarro.XXXXXX")
trap 'rm -f "$staged"' EXIT
install -m 755 "$binary" "$staged"
mv -f "$staged" "$bin_dir/quarro"

desktop_dir="$HOME/.local/share/applications"
icon_root="$HOME/.local/share/icons/hicolor"
mkdir -p "$desktop_dir"
install -m 644 build/linux/quarro.desktop "$desktop_dir/quarro.desktop"
for icon in brand/desktop/linux/hicolor/*/apps/quarro*.png; do
    size_dir=${icon#brand/desktop/linux/hicolor/}
    install -D -m 644 "$icon" "$icon_root/$size_dir"
done

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$desktop_dir" >/dev/null 2>&1 || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q "$HOME/.local/share/icons/hicolor" >/dev/null 2>&1 || true
fi

echo "Installed $stage build to $bin_dir/quarro"
echo "Installed desktop entry to $desktop_dir/quarro.desktop"
