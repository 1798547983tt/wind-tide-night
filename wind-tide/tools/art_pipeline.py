"""Build the web artwork set for 风与潮之夜 (Night of Wind & Tide).

Artwork never enters the public repository (docs/adr/0003). This script reads
the original illustrations from a local folder and writes web-ready files to
`wind-tide/.serve/art/`, which is gitignored, served at `/art/` by the dev
server and uploaded to the production server's art directory.

For each image it writes resized AVIF and WebP copies, and optionally a depth
map (white = near) used by the 2.5D parallax painting. Depth comes from the
Depth Anything V2 Small ONNX model (Apache-2.0). A line drawing on plain white
paper, such as the mascot, can be cut out onto a transparent background.

Full-screen paintings whose source is too small for large or high-density
screens are first enlarged 4x with the APISR GRL anime super-resolution model
(ONNX export Xenova/4x_APISR_GRL_GAN_generator-onnx). Enlarged copies are
cached in `wind-tide/.cache/upscaled/`, which is gitignored as well.

Usage:
  python wind-tide/tools/art_pipeline.py --src <folder with 1.png..> \
      --model <depth-anything-v2-small/model.onnx> \
      [--sr-model <4x_APISR_GRL/model.onnx>] [--only stage,hero-day]

Requires: pillow (with AVIF), numpy, onnxruntime.
"""

from __future__ import annotations

import argparse
import json
from dataclasses import dataclass, field
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw, ImageFilter, ImageOps

OUT_DIR = Path(__file__).resolve().parent.parent / ".serve" / "art"
CACHE_DIR = Path(__file__).resolve().parent.parent / ".cache" / "upscaled"
MEAN = np.array([0.485, 0.456, 0.406], dtype=np.float32)
STD = np.array([0.229, 0.224, 0.225], dtype=np.float32)

# Super-resolution: 4x, run on overlapping tiles padded to the model's window.
SR_SCALE, SR_TILE, SR_PAD, SR_MULTIPLE = 4, 192, 16, 64
# Sources at least this wide are sharp enough as they are.
UPSCALE_BELOW = 1600
# Widest copy shipped, and widest depth map (depth is smooth, so it stays small).
FULL_WIDTH = 2880
DEPTH_WIDTH = 1280


@dataclass
class Artwork:
    name: str
    source: str
    widths: list[int]
    depth: bool = False
    # Prefer a processed or GPT-made source when it exists.
    alternates: list[str] = field(default_factory=list)
    # Enlarge a small source with the super-resolution model first.
    upscale: bool = False
    # Lift a line drawing off its white paper onto a transparent background.
    cutout: bool = False
    # Part of the source to keep, as (left, top, right, bottom) pixels.
    crop: tuple[int, int, int, int] | None = None
    # Paper the outline closes in, such as a gap between strands of hair, as
    # (x, y) points inside each gap, counted after the crop.
    holes: tuple[tuple[int, int], ...] = ()


ARTWORKS = [
    Artwork(
        "stage", "1.png", [720, 1080, 1600, 2160], depth=True, alternates=["gpt/A1.png"], upscale=True
    ),
    Artwork(
        "hero-day", "11.png", [720, 1080, 1600, 2160], depth=True, alternates=["gpt/B1.png"], upscale=True
    ),
    Artwork("hero-night", "gpt/B2.png", [720, 1080, 1600, 2160], depth=True, upscale=True),
    *[Artwork(f"gallery-{i}", f"{i}.png", [480, 960]) for i in (2, 3, 5, 6, 7, 8, 9, 10)],
    # Full-height panels beside the sign-in and the entry-request pages.
    Artwork("auth-welcome", "6.png", [720, 1080, 1600], upscale=True),
    Artwork("auth-tea", "9.png", [720, 1080, 1600], upscale=True),
    # The mascot, without the stray pencil marks in the margins of 4.png.
    Artwork(
        "mascot-base",
        "4.png",
        [160, 320, 480],
        cutout=True,
        crop=(180, 400, 979, 1217),
        holes=((123, 273), (24, 711)),
    ),
]


def pick_source(src_dir: Path, art: Artwork) -> Path | None:
    for candidate in [*art.alternates, art.source]:
        path = src_dir / candidate
        if path.exists():
            return path
    return None


def save_sizes(image: Image.Image, art: Artwork) -> list[int]:
    written = []
    for width in art.widths:
        if width > image.width * 1.05:
            continue
        height = round(image.height * width / image.width)
        resized = image.resize((width, height), Image.Resampling.LANCZOS)
        resized.save(OUT_DIR / f"{art.name}-{width}.avif", quality=62, speed=4)
        resized.save(OUT_DIR / f"{art.name}-{width}.webp", quality=84, method=6)
        written.append(width)
    # Always keep a copy at the native width for the largest screens.
    image.save(OUT_DIR / f"{art.name}-full.webp", quality=86, method=6)
    return written


def upscale(session, image: Image.Image) -> Image.Image:
    """Enlarge 4x with the anime super-resolution model, tile by tile."""
    arr = np.asarray(image, dtype=np.float32) / 255.0
    h, w, _ = arr.shape
    out = np.zeros((h * SR_SCALE, w * SR_SCALE, 3), np.float32)
    for y0 in range(0, h, SR_TILE):
        for x0 in range(0, w, SR_TILE):
            y1, x1 = min(y0 + SR_TILE, h), min(x0 + SR_TILE, w)
            # Each tile sees a margin of its neighbours so seams do not show.
            cy0, cx0 = max(y0 - SR_PAD, 0), max(x0 - SR_PAD, 0)
            cy1, cx1 = min(y1 + SR_PAD, h), min(x1 + SR_PAD, w)
            patch = arr[cy0:cy1, cx0:cx1]
            ph, pw = patch.shape[:2]
            th = -(-ph // SR_MULTIPLE) * SR_MULTIPLE
            tw = -(-pw // SR_MULTIPLE) * SR_MULTIPLE
            mode = "reflect" if th - ph < ph and tw - pw < pw else "edge"
            patch = np.pad(patch, ((0, th - ph), (0, tw - pw), (0, 0)), mode=mode)
            result = session.run(None, {"pixel_values": patch.transpose(2, 0, 1)[None]})[0][0]
            result = result.transpose(1, 2, 0)
            oy, ox = (y0 - cy0) * SR_SCALE, (x0 - cx0) * SR_SCALE
            out[y0 * SR_SCALE : y1 * SR_SCALE, x0 * SR_SCALE : x1 * SR_SCALE] = result[
                oy : oy + (y1 - y0) * SR_SCALE, ox : ox + (x1 - x0) * SR_SCALE
            ]
    return Image.fromarray((np.clip(out, 0, 1) * 255 + 0.5).astype(np.uint8))


def cut_out(image: Image.Image, holes: tuple[tuple[int, int], ...] = ()) -> Image.Image:
    """Make the white paper around a line drawing transparent.

    Only white that reaches the border is paper, plus the gaps named in
    `holes`; other white inside the outline, such as a dress, stays opaque.
    Pixels on the edge of the paper keep part of their alpha, with the white
    they were blended with taken out of their colour, so the drawing shows no
    pale fringe on a dark page.
    """
    rgb = np.asarray(image, dtype=np.float32) / 255.0
    lightness = rgb.min(axis=2)
    paper = Image.fromarray(((lightness > 0.9) * 255).astype(np.uint8), "L")
    # A white frame lets the fill travel all the way round the drawing.
    framed = ImageOps.expand(paper, border=1, fill=255)
    ImageDraw.floodfill(framed, (0, 0), 128, thresh=0)
    for x, y in holes:
        if framed.getpixel((x + 1, y + 1)) != 255:
            raise ValueError(f"hole ({x}, {y}) is not on enclosed paper")
        ImageDraw.floodfill(framed, (x + 1, y + 1), 128, thresh=0)
    background = np.asarray(framed)[1:-1, 1:-1] == 128
    # One pixel of the outline's outer side is anti-aliasing against the paper.
    edge = np.asarray(Image.fromarray(background.astype(np.uint8) * 255).filter(ImageFilter.MaxFilter(3))) > 0
    whiteness = np.clip((lightness - 0.55) / 0.45, 0, 1)
    alpha = np.where(background, 0.0, np.where(edge, 1.0 - whiteness, 1.0))
    a = np.maximum(alpha, 1e-3)[..., None]
    colour = np.clip((rgb - (1 - a)) / a, 0, 1)
    rgba = np.dstack([colour, alpha])
    result = Image.fromarray((rgba * 255 + 0.5).astype(np.uint8), "RGBA")
    return result.crop(result.getbbox())


def estimate_depth(session, image: Image.Image) -> Image.Image:
    short = 518
    if image.width <= image.height:
        w = short
        h = round(image.height * short / image.width / 14) * 14
    else:
        h = short
        w = round(image.width * short / image.height / 14) * 14
    rgb = np.asarray(image.resize((w, h), Image.Resampling.BICUBIC), dtype=np.float32) / 255.0
    tensor = ((rgb - MEAN) / STD).transpose(2, 0, 1)[None].astype(np.float32)
    depth = session.run(None, {"pixel_values": tensor})[0][0]
    lo, hi = np.percentile(depth, [2, 98])
    depth = np.clip((depth - lo) / max(hi - lo, 1e-6), 0, 1)
    out = Image.fromarray((depth * 255).astype(np.uint8), "L")
    width = min(image.width, DEPTH_WIDTH)
    out = out.resize((width, round(image.height * width / image.width)), Image.Resampling.BICUBIC)
    # Grow near regions slightly, then soften, so edges do not tear when the
    # painting shifts with the pointer.
    out = out.filter(ImageFilter.MaxFilter(7)).filter(ImageFilter.GaussianBlur(3))
    return out


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--src", required=True, type=Path)
    parser.add_argument("--model", type=Path)
    parser.add_argument("--sr-model", type=Path)
    parser.add_argument("--only", default="")
    args = parser.parse_args()

    only = {name for name in args.only.split(",") if name}
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    manifest_path = OUT_DIR / "manifest.json"
    manifest = json.loads(manifest_path.read_text("utf-8")) if manifest_path.exists() else {}
    session = None
    sr_session = None
    if args.model or args.sr_model:
        import onnxruntime as ort

        if args.model:
            session = ort.InferenceSession(str(args.model), providers=["CPUExecutionProvider"])
        if args.sr_model:
            sr_session = ort.InferenceSession(str(args.sr_model), providers=["CPUExecutionProvider"])

    for art in ARTWORKS:
        if only and art.name not in only:
            continue
        path = pick_source(args.src, art)
        if path is None:
            print(f"skip {art.name}: no source")
            continue
        image = Image.open(path).convert("RGB")
        if art.crop:
            image = image.crop(art.crop)
        if art.cutout:
            image = cut_out(image, art.holes)
        if art.upscale and sr_session is not None and image.width < UPSCALE_BELOW:
            cached = CACHE_DIR / f"{art.name}-{path.stem}-{int(path.stat().st_mtime)}.png"
            if not cached.exists():
                CACHE_DIR.mkdir(parents=True, exist_ok=True)
                upscale(sr_session, image).save(cached)
            image = Image.open(cached).convert("RGB")
        if image.width > FULL_WIDTH:
            height = round(image.height * FULL_WIDTH / image.width)
            image = image.resize((FULL_WIDTH, height), Image.Resampling.LANCZOS)
        widths = save_sizes(image, art)
        has_depth = art.depth and session is not None
        if has_depth:
            estimate_depth(session, image).save(OUT_DIR / f"{art.name}-depth.webp", lossless=True)
        # The frontend reads this to pick a size and to lay out before loading.
        manifest[art.name] = {
            "width": image.width,
            "height": image.height,
            "sizes": widths,
            "depth": has_depth or bool(manifest.get(art.name, {}).get("depth")),
        }
        print(f"done {art.name} from {path.name} {image.size}")

    manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", "utf-8")


if __name__ == "__main__":
    main()
