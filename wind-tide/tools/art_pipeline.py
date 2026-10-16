"""Build the web artwork set for 风与潮之夜 (Night of Wind & Tide).

Artwork never enters the public repository (docs/adr/0003). This script reads
the original illustrations from a local folder and writes web-ready files to
`wind-tide/.serve/art/`, which is gitignored, served at `/art/` by the dev
server and uploaded to the production server's art directory.

For each image it writes resized AVIF and WebP copies, and optionally a depth
map (white = near) used by the 2.5D parallax painting. Depth comes from the
Depth Anything V2 Small ONNX model (Apache-2.0). A line drawing on plain white
paper, such as the mascot, can be cut out onto a transparent background; a
sticker sheet drawn on a transparent background keeps it. The theatre curtain
is split into its two halves and the pelmet that hangs across them.

Full-screen paintings whose source is too small for large or high-density
screens are first enlarged 4x with the APISR GRL anime super-resolution model
(ONNX export Xenova/4x_APISR_GRL_GAN_generator-onnx). Enlarged copies are
cached in `wind-tide/.cache/upscaled/`, which is gitignored as well.

Usage:
  python wind-tide/tools/art_pipeline.py --src <folder with 1.png..> \
      [--gpt <folder with the GPT-made A1.png..; default <src>/gpt>] \
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
from numpy.lib.stride_tricks import sliding_window_view
from PIL import Image, ImageDraw, ImageFilter, ImageOps

OUT_DIR = Path(__file__).resolve().parent.parent / ".serve" / "art"
CACHE_DIR = Path(__file__).resolve().parent.parent / ".cache" / "upscaled"
MEAN = np.array([0.485, 0.456, 0.406], dtype=np.float32)
STD = np.array([0.229, 0.224, 0.225], dtype=np.float32)

# Super-resolution: 4x, run on overlapping tiles padded to the model's window.
SR_SCALE, SR_TILE, SR_PAD, SR_MULTIPLE = 4, 192, 16, 64
# Sources at least this wide are sharp enough as they are: about what a
# full-screen painting needs on a large screen.
UPSCALE_BELOW = 2400
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
    # The source already has a transparent background: keep it, and trim the
    # picture to what shows.
    alpha: bool = False
    # Keep only the pelmet along the top of a curtain painting; what hangs
    # below its gold fringe becomes transparent.
    valance: bool = False
    # Part of the source to keep, as (left, top, right, bottom): pixels, or
    # fractions of the source's size when given as floats.
    crop: tuple[int, int, int, int] | tuple[float, float, float, float] | None = None
    # Paper the outline closes in, such as a gap between strands of hair, as
    # (x, y) points inside each gap, counted after the crop.
    holes: tuple[tuple[int, int], ...] = ()


ARTWORKS = [
    # The opening: 1.png fills a phone held upright, A1 is the same scene
    # widened for landscape screens, and A2 and A3 are the two by moonlight.
    Artwork("stage", "1.png", [720, 1080, 1600, 2160], depth=True, upscale=True),
    Artwork("stage-wide", "gpt/A1.png", [720, 1080, 1600, 2160], depth=True, upscale=True),
    Artwork("stage-night", "gpt/A2.png", [720, 1080, 1600, 2160], depth=True, upscale=True),
    Artwork("stage-night-wide", "gpt/A3.png", [720, 1080, 1600, 2160], depth=True, upscale=True),
    Artwork(
        "hero-day", "11.png", [720, 1080, 1600, 2160], depth=True, alternates=["gpt/B1.png"], upscale=True
    ),
    Artwork("hero-night", "gpt/B2.png", [720, 1080, 1600, 2160], depth=True, upscale=True),
    *[Artwork(f"gallery-{i}", f"{i}.png", [480, 960]) for i in (2, 5, 6, 7, 8, 9, 10)],
    # D is 3.png with its lettering painted out.
    Artwork("gallery-3", "3.png", [480, 960], alternates=["gpt/D.png"]),
    # The theatre curtain (E): the halves part at its centre seam, and the
    # pelmet stays across the top. The halves start below the lowest tassel
    # of the fringe, so no piece of pelmet gathers to the sides with them.
    Artwork("curtain-left", "gpt/E.png", [480, 960, 1440, 1920], upscale=True, crop=(0.0, 0.14, 0.5, 1.0)),
    Artwork("curtain-right", "gpt/E.png", [480, 960, 1440, 1920], upscale=True, crop=(0.5, 0.14, 1.0, 1.0)),
    Artwork(
        "curtain-valance", "gpt/E.png", [1080, 1600, 2160], upscale=True, crop=(0.0, 0.0, 1.0, 0.2), valance=True
    ),
    # The mascot's expressions (C): one sticker in each cell of a 3 x 2 sheet,
    # in the order wind-tide/ASSETS.md asks for them.
    *[
        Artwork(
            f"mascot-{mood}",
            "gpt/C.png",
            [160, 320, 480],
            alpha=True,
            crop=((i % 3) / 3, (i // 3) / 2, (i % 3 + 1) / 3, (i // 3 + 1) / 2),
        )
        for i, mood in enumerate(("wave", "rich", "sleep", "cry", "think", "toast"))
    ],
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

# Where the subject sits in each source, as fractions of the finished image,
# written into the manifest with the files: `focus` is the point a screen of
# another shape keeps in view, `faces` where the opening's camera pushes in,
# and `horizon` the sea line the night grade of a day painting starts from.
FRAMING = {
    "1.png": {"focus": [0.5, 0.27], "faces": [0.49, 0.235]},
    "A1.png": {"focus": [0.49, 0.4], "faces": [0.485, 0.3]},
    "A2.png": {"focus": [0.48, 0.5], "faces": [0.478, 0.35]},
    "A3.png": {"focus": [0.49, 0.4], "faces": [0.485, 0.3]},
    "11.png": {"focus": [0.55, 0.3], "horizon": 0.9},
    "B1.png": {"focus": [0.58, 0.31], "horizon": 0.75},
    "B2.png": {"focus": [0.42, 0.33], "horizon": 0.61},
}


def pick_source(src_dir: Path, gpt_dir: Path, art: Artwork) -> Path | None:
    for candidate in [*art.alternates, art.source]:
        name = candidate.removeprefix("gpt/")
        path = gpt_dir / name if name != candidate else src_dir / candidate
        if path.exists():
            return path
    return None


def crop_box(crop: tuple, size: tuple[int, int]) -> tuple[int, int, int, int]:
    """A crop in pixels, from pixels or from fractions of `size`."""
    scale = (size[0], size[1], size[0], size[1])
    return tuple(round(v * s) if isinstance(v, float) else v for v, s in zip(crop, scale))


def trim_transparent(image: Image.Image, margin: int = 8) -> Image.Image:
    """Clear near-invisible specks, then crop to what shows plus a margin."""
    rgba = np.array(image)
    rgba[..., 3][rgba[..., 3] < 16] = 0
    result = Image.fromarray(rgba, "RGBA")
    left, top, right, bottom = result.getchannel("A").getbbox()
    return result.crop(
        (
            max(left - margin, 0),
            max(top - margin, 0),
            min(right + margin, result.width),
            min(bottom + margin, result.height),
        )
    )


def cut_valance(image: Image.Image) -> Image.Image:
    """Keep the pelmet across the top of a curtain painting, nothing below it.

    The pelmet ends at its gold fringe: in each column, the lowest gold pixel,
    carried across the gaps between tassels and smoothed. The edge is found at
    about the size the painting was made at, where the tassels are a few
    pixels wide, and then drawn at full size with a one-pixel soft edge.

    Lit folds of the velvet below the fringe pass for gold too, as thin lines
    well below every tassel around them. A column whose gold reaches more
    than 10 pixels below all gold in the 20 columns on either side, leaving
    out its 3 closest neighbours on each side, takes the edge of those
    columns instead. Tassels come every few columns, so a real one always has
    others about as low nearby.
    """
    probe_width = min(image.width, 1672)
    scale = image.width / probe_width
    probe = image.convert("RGB").resize((probe_width, max(round(image.height / scale), 1)), Image.Resampling.BOX)
    rgb = np.asarray(probe, dtype=np.int16)
    r, g, b = rgb[..., 0], rgb[..., 1], rgb[..., 2]
    # Warm and bright, with far more green than the red velvet has.
    gold = (r > 140) & (g > 95) & (g - b > 40) & (r - g < 110)
    rows = np.arange(gold.shape[0])[:, None]
    edge = np.where(gold, rows, 0).max(axis=0).astype(np.float32)
    around = sliding_window_view(np.pad(edge, 20), 41).copy()
    around[:, 17:24] = 0
    around = around.max(axis=1)
    edge = np.where(edge > around + 10, around, edge)
    edge = sliding_window_view(np.pad(edge, 8, mode="edge"), 17).max(axis=1)
    edge = np.convolve(np.pad(edge, 4, mode="edge"), np.ones(9) / 9, mode="valid")
    xs = (np.arange(image.width) + 0.5) / scale - 0.5
    edge = (np.interp(xs, np.arange(probe_width), edge) + 1) * scale
    y = np.arange(image.height, dtype=np.float32)[:, None]
    alpha = np.clip(edge[None, :] - y, 0, 1)
    rgba = np.dstack([np.asarray(image.convert("RGB")), (alpha * 255 + 0.5).astype(np.uint8)])
    bottom = min(image.height, int(np.ceil(edge.max())) + 1)
    return Image.fromarray(rgba, "RGBA").crop((0, 0, image.width, bottom))


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
    parser.add_argument("--gpt", type=Path, help="folder with the GPT-made images; default <src>/gpt")
    parser.add_argument("--model", type=Path)
    parser.add_argument("--sr-model", type=Path)
    parser.add_argument("--only", default="")
    args = parser.parse_args()

    only = {name for name in args.only.split(",") if name}
    gpt_dir = args.gpt or args.src / "gpt"
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
        path = pick_source(args.src, gpt_dir, art)
        if path is None:
            print(f"skip {art.name}: no source")
            continue
        image = Image.open(path).convert("RGBA" if art.alpha else "RGB")
        if art.crop:
            image = image.crop(crop_box(art.crop, image.size))
        if art.alpha:
            image = trim_transparent(image)
        if art.cutout:
            image = cut_out(image, art.holes)
        if art.upscale and sr_session is not None and image.width < UPSCALE_BELOW:
            key = f"{art.name}-{path.stem}-{int(path.stat().st_mtime)}"
            if art.crop:
                key += "-" + "-".join(f"{v:g}" for v in art.crop)
            cached = CACHE_DIR / f"{key}.png"
            if not cached.exists():
                CACHE_DIR.mkdir(parents=True, exist_ok=True)
                upscale(sr_session, image).save(cached)
            image = Image.open(cached).convert("RGB")
        if art.valance:
            image = cut_valance(image)
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
            **FRAMING.get(path.name, {}),
        }
        print(f"done {art.name} from {path.name} {image.size}")

    manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", "utf-8")


if __name__ == "__main__":
    main()
