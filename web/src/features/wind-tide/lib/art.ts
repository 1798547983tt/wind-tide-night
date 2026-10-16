/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/

/**
 * Artwork is served by the deployment at /art/ and never bundled with the
 * app (docs/adr/0003). wind-tide/tools/art_pipeline.py builds the files
 * and a manifest listing which widths exist for each illustration.
 */
export const ART_BASE = '/art'

export type ArtEntry = {
  width: number
  height: number
  sizes: number[]
  depth: boolean
  /**
   * Where the subject sits, as fractions of the image: the point a screen of
   * another shape keeps in view, where the opening's camera pushes in, and
   * the sea line a day painting's night grade starts from. The pipeline
   * writes them for the source it used.
   */
  focus?: [number, number]
  faces?: [number, number]
  horizon?: number
}

export type ArtManifest = Record<string, ArtEntry>

// Opening paintings to try for each time of day and screen shape: a
// moonlit one before a day one, then the other shape.
const STAGE_ORDER = {
  day: {
    wide: ['stage-wide', 'stage'],
    upright: ['stage', 'stage-wide'],
  },
  night: {
    wide: ['stage-night-wide', 'stage-night', 'stage-wide', 'stage'],
    upright: ['stage-night', 'stage-night-wide', 'stage', 'stage-wide'],
  },
}

/** The opening painting the manifest offers for this time and screen. */
export function stageArtName(
  manifest: ArtManifest | undefined,
  night: boolean,
  wide: boolean
) {
  const order = STAGE_ORDER[night ? 'night' : 'day'][wide ? 'wide' : 'upright']
  return order.find((name) => manifest?.[name]) ?? 'stage'
}

export function artFile(
  name: string,
  variant: number | 'full' | 'depth',
  ext: 'webp' | 'avif' = 'webp'
) {
  return `${ART_BASE}/${name}-${variant}.${ext}`
}

/** `srcset` over the widths the manifest lists, plus the full-size copy. */
export function artSrcSet(
  name: string,
  entry: ArtEntry | undefined,
  ext: 'webp' | 'avif'
) {
  if (!entry) return undefined
  const sizes = entry.sizes.map(
    (width) => `${artFile(name, width, ext)} ${width}w`
  )
  if (ext === 'webp') sizes.push(`${artFile(name, 'full')} ${entry.width}w`)
  return sizes.join(', ')
}

/**
 * Pixels wide a painting must be to fill the viewport's height and `share` of
 * its width the way `object-fit: cover` crops it, at the painting canvas's
 * density (capped at 1.75). On a tall phone screen a landscape painting is
 * enlarged to fill the height, so it needs far more pixels than the screen
 * is wide.
 */
export function coverWidth(entry: ArtEntry | undefined, share = 1) {
  if (typeof window === 'undefined') return 1600
  const aspect = entry ? entry.width / entry.height : 16 / 9
  const dpr = Math.min(window.devicePixelRatio || 1, 1.75)
  return Math.max(window.innerWidth * share, window.innerHeight * aspect) * dpr
}

/**
 * Smallest listed width that covers `needed` pixels, else the full copy.
 * WebGL textures use WebP, which every target browser decodes.
 */
export function pickArtUrl(
  name: string,
  entry: ArtEntry | undefined,
  needed: number
) {
  if (!entry) return artFile(name, 'full')
  const width = entry.sizes.find((size) => size >= needed)
  return width ? artFile(name, width) : artFile(name, 'full')
}
