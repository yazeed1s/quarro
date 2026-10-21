#!/usr/bin/env python3
"""Generate Quarro desktop, website/PWA, and in-app icon assets.

Requirements:
    python -m pip install Pillow

Expected input files (original PNGs, with transparency):
    brand/source/quarro-light.png
    brand/source/quarro-dark.png

Usage:
    python brand/generate_icons.py
    python brand/generate_icons.py --default-theme light
    python brand/generate_icons.py --light ./other-light.png --dark ./other-dark.png

No SVG output is created: embedding a PNG in an SVG does not make it a vector.
For genuinely resolution-independent header icons, obtain/create vector originals.
"""

from __future__ import annotations

import argparse
import json
import shutil
import struct
from pathlib import Path

from PIL import Image

BRAND = Path(__file__).resolve().parent
# go:embed cannot reach ../../brand, so the app icons internal/desktop swaps
# at runtime are copied beside it.
RUNTIME_ICONS = BRAND.parent / "internal" / "desktop" / "icons"
THEMES = ("light", "dark")
BG = {"light": "#f0f1f1", "dark": "#181919"}
DESKTOP_ICO_SIZES = (16, 24, 32, 48, 64, 128, 256)
ICNS_ENTRIES = {
    16: b"icp4",
    32: b"icp5",
    64: b"icp6",
    128: b"ic07",
    256: b"ic08",
    512: b"ic09",
    1024: b"ic10",
}
HEADER_SIZES = (16, 20, 24, 32)
HEADER_SCALE_FACTORS = (1, 2, 3)
LINUX_SIZES = (16, 22, 24, 32, 48, 64, 128, 256, 512)


def fit_and_center(image: Image.Image, canvas_size: int = 1024, occupancy: float = 0.90) -> Image.Image:
    """Center real visible content, removing inconsistent transparent source margins."""
    image = image.convert("RGBA")
    alpha = image.getchannel("A").point(lambda a: 255 if a >= 40 else 0)
    bbox = alpha.getbbox()
    if bbox is None:
        raise ValueError("Source icon contains no visible pixels")

    cropped = image.crop(bbox)
    target = round(canvas_size * occupancy)
    factor = target / max(cropped.size)
    new_size = tuple(max(1, round(dim * factor)) for dim in cropped.size)
    cropped = cropped.resize(new_size, Image.Resampling.LANCZOS)
    result = Image.new("RGBA", (canvas_size, canvas_size), (0, 0, 0, 0))
    x = (canvas_size - new_size[0]) // 2
    y = (canvas_size - new_size[1]) // 2
    result.paste(cropped, (x, y))
    return result


def png(image: Image.Image, path: Path, size: int | None = None) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if size is not None:
        image = image.resize((size, size), Image.Resampling.LANCZOS)
    image.save(path, format="PNG", optimize=True)


def ico(image: Image.Image, path: Path, sizes: tuple[int, ...]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    # Pillow embeds each size as an independent entry in the ICO container.
    image.save(path, format="ICO", sizes=[(s, s) for s in sizes])


def icns(image: Image.Image, path: Path) -> None:
    """Write PNG-backed Apple .icns (no macOS-only iconutil required)."""
    from io import BytesIO

    records = []
    for size, code in ICNS_ENTRIES.items():
        buffer = BytesIO()
        image.resize((size, size), Image.Resampling.LANCZOS).save(buffer, "PNG")
        blob = buffer.getvalue()
        records.append(code + struct.pack(">I", len(blob) + 8) + blob)

    # Additional high-DPI type codes for Retina icon display.
    for size, code in ((32, b"ic11"), (64, b"ic12"),
                       (256, b"ic13"), (512, b"ic14")):
        buffer = BytesIO()
        image.resize((size, size), Image.Resampling.LANCZOS).save(buffer, "PNG")
        blob = buffer.getvalue()
        records.append(code + struct.pack(">I", len(blob) + 8) + blob)

    payload = b"".join(records)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(b"icns" + struct.pack(">I", len(payload) + 8) + payload)


def filled(image: Image.Image, color: str, size: int) -> Image.Image:
    """Flatten against a solid background, as useful for Apple touch icons."""
    layer = image.resize((size, size), Image.Resampling.LANCZOS)
    out = Image.new("RGBA", (size, size), color)
    out.alpha_composite(layer)
    return out.convert("RGB")


def maskable(image: Image.Image, color: str, size: int = 512) -> Image.Image:
    """Opaque full-bleed background and extra padding for PWA masks."""
    out = Image.new("RGBA", (size, size), color)
    # Keep the actual mark well inside the central safe-zone circle.
    inner_size = round(size * 0.74)
    layer = image.resize((inner_size, inner_size), Image.Resampling.LANCZOS)
    offset = (size - inner_size) // 2
    out.alpha_composite(layer, (offset, offset))
    return out.convert("RGB")


def desktop_assets(master: Image.Image, directory: Path) -> None:
    png(master, directory / "icon.png", 512)
    png(master, directory / "icon-1024.png", 1024)
    ico(master, directory / "icon.ico", DESKTOP_ICO_SIZES)
    icns(master, directory / "icon.icns")


def web_assets(master: Image.Image, directory: Path, theme: str) -> None:
    for size in (16, 32, 48):
        png(master, directory / f"favicon-{size}x{size}.png", size)
    ico(master, directory / "favicon.ico", (16, 32, 48))
    png(filled(master, BG[theme], 180), directory / "apple-touch-icon.png")
    png(master, directory / "pwa-192.png", 192)
    png(master, directory / "pwa-512.png", 512)
    png(maskable(master, BG[theme], 512), directory / "pwa-maskable-512.png")


def header_assets(master: Image.Image, theme: str, directory: Path) -> None:
    for size in HEADER_SIZES:
        for scale in HEADER_SCALE_FACTORS:
            suffix = f"@{scale}x" if scale > 1 else ""
            # The size number is intended CSS px, and @Nx is the pixel density.
            png(master, directory / f"header-{theme}-{size}{suffix}.png", size * scale)
    # Convenient default for a 24 CSS px header icon on a 2x display.
    png(master, directory / f"header-{theme}.png", 48)


def build_manifest(default_theme: str, directory: Path) -> None:
    # The manifest is an example ready to copy into the web app's public root.
    # Adjust start_url and scope if Quarro's deployed site needs something else.
    manifest = {
        "name": "Quarro",
        "short_name": "Quarro",
        "start_url": "/",
        "display": "standalone",
        "background_color": BG[default_theme],
        "theme_color": BG[default_theme],
        "icons": [
            {"src": "pwa-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any"},
            {"src": "pwa-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any"},
            {"src": "pwa-maskable-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable"},
        ],
    }
    (directory / "manifest.webmanifest").write_text(json.dumps(manifest, indent=2) + "\n")


def generate(inputs: dict[str, Path], default_theme: str) -> None:
    masters: dict[str, Image.Image] = {}
    for theme in THEMES:
        if not inputs[theme].is_file():
            raise FileNotFoundError(f"Missing {theme} input: {inputs[theme]}")
        with Image.open(inputs[theme]) as image:
            masters[theme] = fit_and_center(image)

    for theme, master in masters.items():
        desktop_assets(master, BRAND / "desktop" / theme)
        web_assets(master, BRAND / "web" / theme, theme)
        header_assets(master, theme, BRAND / "ui")

    # Stable names: a single default icon for installers and standard site assets.
    for name in ("icon.png", "icon-1024.png", "icon.ico", "icon.icns"):
        shutil.copy2(BRAND / "desktop" / default_theme / name, BRAND / "desktop" / name)

    for name in ("favicon.ico", "favicon-16x16.png", "favicon-32x32.png",
                 "favicon-48x48.png", "apple-touch-icon.png", "pwa-192.png",
                 "pwa-512.png", "pwa-maskable-512.png"):
        shutil.copy2(BRAND / "web" / default_theme / name, BRAND / "web" / name)

    RUNTIME_ICONS.mkdir(parents=True, exist_ok=True)
    for theme in THEMES:
        shutil.copy2(BRAND / "desktop" / theme / "icon-1024.png", RUNTIME_ICONS / f"{theme}.png")
        shutil.copy2(BRAND / "desktop" / theme / "icon.ico", RUNTIME_ICONS / f"{theme}.ico")

    build_manifest(default_theme, BRAND / "web")
    # "quarro" is what the .desktop file ships with; the named variants let
    # the App icon setting repoint the user's launcher entry on Linux.
    for size in LINUX_SIZES:
        apps = BRAND / "desktop" / "linux" / "hicolor" / f"{size}x{size}" / "apps"
        png(masters[default_theme], apps / "quarro.png", size)
        for theme, master in masters.items():
            png(master, apps / f"quarro-{theme}.png", size)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--light", type=Path, default=BRAND / "source" / "quarro-light.png")
    parser.add_argument("--dark", type=Path, default=BRAND / "source" / "quarro-dark.png")
    parser.add_argument("--default-theme", choices=THEMES, default="dark")
    args = parser.parse_args()
    generate({"light": args.light, "dark": args.dark}, args.default_theme)
    print(f"Generated Quarro assets under: {BRAND}")
    print(f"Default desktop/web theme: {args.default_theme}")
    print("UI PNGs are exported at 1x/2x/3x. No fake SVG files generated.")


if __name__ == "__main__":
    main()
