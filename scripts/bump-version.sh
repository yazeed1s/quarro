#!/usr/bin/env bash
# set the release version in build/config.yml and every packaged asset that
# carries its own copy. internal/app's TestBuildAssetsMatchConfigVersion
# fails if they drift.
#
# Unlike `wails3 task common:update:build-assets`, this only touches the
# version fields, so hand edits to the assets survive.
set -euo pipefail

usage() {
    echo "Usage: $0 MAJOR.MINOR.PATCH[-prerelease]"
}

if [ "$#" -ne 1 ]; then
    usage >&2
    exit 2
fi
version="$1"
if ! [[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$ ]]; then
    usage >&2
    exit 2
fi
# Bundle versions, Windows file versions, and NSIS only accept numbers.
core="${version%%-*}"

cd "$(dirname "$0")/.."

replace() {
    local file="$1" expr="$2"
    perl -0pi -e "$expr" "$file"
}

replace build/config.yml "s/(\\n  version: \")[^\"]*\"/\${1}$version\"/"
for plist in build/darwin/Info.plist build/darwin/Info.dev.plist; do
    replace "$plist" "s/(CFBundleShortVersionString<\\/key>\\s*<string>)[^<]*/\${1}$core/"
    replace "$plist" "s/(CFBundleVersion<\\/key>\\s*<string>)[^<]*/\${1}$core/"
done
replace build/windows/info.json "s/(\"file_version\":\\s*\")[^\"]*/\${1}$core/"
replace build/windows/info.json "s/(\"ProductVersion\":\\s*\")[^\"]*/\${1}$version/"
replace build/windows/wails.exe.manifest "s/(assemblyIdentity[^>]*version=\")[^\"]*/\${1}$core/"
replace build/windows/nsis/wails_tools.nsh "s/(!define INFO_PRODUCTVERSION \")[^\"]*/\${1}$core/"
replace build/linux/nfpm/nfpm.yaml "s/(\\nversion: \")[^\"]*/\${1}$version/"

go test ./internal/app/ -run TestBuildAssetsMatchConfigVersion >/dev/null
echo "version set to $version"
