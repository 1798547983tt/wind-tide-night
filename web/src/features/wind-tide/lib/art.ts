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
}

export type ArtManifest = Record<string, ArtEntry>

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
 * Pixels wide a painting must be to fill the viewport the way `object-fit:
 * cover` crops it, at the painting canvas's density (capped at 1.75). On a
 * tall phone screen a landscape painting is enlarged to fill the height, so
 * it needs far more pixels than the screen is wide.
 */
export function coverWidth(entry: ArtEntry | undefined) {
  if (typeof window === 'undefined') return 1600
  const aspect = entry ? entry.width / entry.height : 16 / 9
  const dpr = Math.min(window.devicePixelRatio || 1, 1.75)
  return Math.max(window.innerWidth, window.innerHeight * aspect) * dpr
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
