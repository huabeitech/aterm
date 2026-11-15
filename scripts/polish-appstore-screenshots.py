#!/usr/bin/env python3
from __future__ import annotations

from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont


CANVAS_SIZE = (2880, 1800)
INPUT_DIR = Path("build/appstore-upload")
OUTPUT_DIR = Path("build/appstore-upload-polished")
WALLPAPER_PATH = OUTPUT_DIR / "15-Sequoia-Light-6K.jpg"
CONTENT_X = 260
SOURCE_TOP_PADDING = 84
SOURCE_BOTTOM_PADDING = 82

TITLES = {
    "01-hosts-workspace.png": "Manage SSH Hosts",
    "02-host-search.png": "Find Connections Fast",
    "03-local-terminal.png": "Work in Local Terminal",
    "04-settings.png": "Configure Securely",
    "05-host-details.png": "Edit Host Details",
    "06-batch-add.png": "Import Hosts in Bulk",
}


def load_font(size: int, weight: str = "regular") -> ImageFont.FreeTypeFont | ImageFont.ImageFont:
    candidates = []
    if weight == "bold":
        candidates.extend(
            [
                "/System/Library/Fonts/SFNS-Bold.ttf",
                "/System/Library/Fonts/Supplemental/Arial Bold.ttf",
            ]
        )
    candidates.extend(
        [
            "/System/Library/Fonts/SFNS.ttf",
            "/System/Library/Fonts/Supplemental/Arial.ttf",
            "/Library/Fonts/Arial.ttf",
        ]
    )
    for candidate in candidates:
        try:
            return ImageFont.truetype(candidate, size)
        except OSError:
            continue
    return ImageFont.load_default()


def fixed_crop_app_area(image: Image.Image) -> Image.Image:
    width, height = image.size
    top = SOURCE_TOP_PADDING
    bottom = height - SOURCE_BOTTOM_PADDING
    if bottom <= top:
        raise ValueError(f"Image is too short for fixed padding crop: {width}x{height}")
    return image.convert("RGBA").crop((0, top, width, bottom))


def fill_transparent_edges(image: Image.Image) -> Image.Image:
    rgba = image.convert("RGBA")
    width, height = rgba.size
    pixels = rgba.load()
    top_fill = rgba.getpixel((width // 2, 8))
    bottom_fill = rgba.getpixel((width // 2, height - 8))
    left_bottom_fill = rgba.getpixel((8, height - 8))

    for y in range(height):
        for x in range(width):
            r, g, b, a = pixels[x, y]
            if a == 255:
                continue

            if y < 96:
                fill = top_fill
            elif x < 420:
                fill = left_bottom_fill
            else:
                fill = bottom_fill

            fr, fg, fb, _ = fill
            alpha = a / 255
            pixels[x, y] = (
                round(r * alpha + fr * (1 - alpha)),
                round(g * alpha + fg * (1 - alpha)),
                round(b * alpha + fb * (1 - alpha)),
                255,
            )

    return rgba


def make_wallpaper_background(size: tuple[int, int]) -> Image.Image:
    if not WALLPAPER_PATH.exists():
        raise FileNotFoundError(f"Wallpaper background not found: {WALLPAPER_PATH}")

    wallpaper = Image.open(WALLPAPER_PATH).convert("RGB")
    scale = max(size[0] / wallpaper.width, size[1] / wallpaper.height)
    resized = wallpaper.resize(
        (round(wallpaper.width * scale), round(wallpaper.height * scale)),
        Image.Resampling.LANCZOS,
    )
    left = (resized.width - size[0]) // 2
    top = (resized.height - size[1]) // 2
    background = resized.crop((left, top, left + size[0], top + size[1]))
    background = background.filter(ImageFilter.GaussianBlur(6))
    veil = Image.new("RGB", size, (246, 248, 250))
    return Image.blend(background, veil, 0.32)


BACKGROUND = make_wallpaper_background(CANVAS_SIZE)


def draw_text(canvas: Image.Image, title: str) -> None:
    draw = ImageDraw.Draw(canvas)
    title_font = load_font(78, "bold")
    caption_font = load_font(34)
    draw.text((CONTENT_X, 178), title, fill=(27, 32, 42), font=title_font)
    draw.text(
        (CONTENT_X + 2, 276),
        "A focused macOS workspace for SSH configuration and connection management.",
        fill=(55, 65, 81),
        font=caption_font,
    )


def polish_one(source: Path, target: Path) -> None:
    original = Image.open(source)
    cropped = fill_transparent_edges(fixed_crop_app_area(original))

    if cropped.mode == "RGBA":
        cropped_rgb = cropped.convert("RGB")
    else:
        cropped_rgb = cropped.convert("RGB")

    canvas = BACKGROUND.copy()
    draw_text(canvas, TITLES.get(source.name, source.stem))

    max_width = 2550
    max_height = 1320
    scale = min(max_width / cropped_rgb.width, max_height / cropped_rgb.height)
    window_size = (round(cropped_rgb.width * scale), round(cropped_rgb.height * scale))
    window = cropped_rgb.resize(window_size, Image.Resampling.LANCZOS)

    x = CONTENT_X
    y = 390

    canvas.paste(window, (x, y))

    target.parent.mkdir(parents=True, exist_ok=True)
    canvas.save(target, "PNG", optimize=True)


def main() -> None:
    if not INPUT_DIR.exists():
        raise SystemExit(f"Input directory not found: {INPUT_DIR}")

    for source in sorted(INPUT_DIR.glob("*.png")):
        polish_one(source, OUTPUT_DIR / source.name)


if __name__ == "__main__":
    main()
