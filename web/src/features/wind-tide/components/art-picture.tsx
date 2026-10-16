/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'

import { cn } from '@/lib/utils'

import { artManifestQueryOptions } from '../api'
import { artFile, artSrcSet } from '../lib/art'

type ArtPictureProps = {
  name: string
  alt: string
  /** The `sizes` attribute: how wide the image renders. */
  sizes: string
  className?: string
  imgClassName?: string
  objectPosition?: string
  /** Load at once and ahead of other images, for a picture above the fold. */
  priority?: boolean
}

/** An illustration from /art/ as AVIF with a WebP fallback, lazily loaded. */
export function ArtPicture(props: ArtPictureProps) {
  const manifest = useQuery(artManifestQueryOptions)
  const entry = manifest.data?.[props.name]

  // Wait for the manifest so the browser picks a size once, not twice.
  if (manifest.isLoading) {
    return <div className={cn('bg-muted/50 block', props.className)} />
  }

  return (
    <picture className={cn('block', props.className)}>
      <source
        type='image/avif'
        srcSet={artSrcSet(props.name, entry, 'avif')}
        sizes={props.sizes}
      />
      <img
        src={artFile(props.name, entry?.sizes.at(-1) ?? 'full')}
        srcSet={artSrcSet(props.name, entry, 'webp')}
        sizes={props.sizes}
        alt={props.alt}
        width={entry?.width}
        height={entry?.height}
        loading={props.priority ? 'eager' : 'lazy'}
        fetchPriority={props.priority ? 'high' : undefined}
        decoding='async'
        className={cn('size-full object-cover', props.imgClassName)}
        style={
          props.objectPosition
            ? { objectPosition: props.objectPosition }
            : undefined
        }
      />
    </picture>
  )
}
