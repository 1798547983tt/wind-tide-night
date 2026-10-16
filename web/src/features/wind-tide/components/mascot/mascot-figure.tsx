/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { artManifestQueryOptions } from '../../api'
import { ArtPicture } from '../art-picture'

/**
 * The mascot's expressions, after the sticker sheet in wind-tide/ASSETS.md
 * (C): waving hello, holding up a bag of coins, asleep, in tears, thinking
 * and eating toast. `base` is the teddy-bear hug from 4.png.
 */
export type MascotMood =
  | 'base'
  | 'wave'
  | 'rich'
  | 'sleep'
  | 'cry'
  | 'think'
  | 'toast'

type MascotFigureProps = {
  mood?: MascotMood
  /** What she says, in a speech bubble. */
  line?: string
  /**
   * Where the bubble sits: above her, or on her left when she stands in a
   * corner of the screen with no room above.
   */
  bubbleSide?: 'above' | 'left'
  /** The `sizes` attribute: how wide the figure renders. */
  sizes: string
  className?: string
  /** Size of the picture itself, such as `size-24`. */
  imageClassName?: string
  /** Makes the picture a button, such as one that asks her to talk. */
  pictureAction?: { label: string; onClick: () => void }
}

/**
 * Chibi Cecilia, the site's mascot (wind-tide/DESIGN.md §8). An expression
 * the deployment has not uploaded yet falls back to the teddy-bear hug, and
 * without any mascot art she stays away, line and all: the line is flavour,
 * and the page around her says what matters in plain words.
 */
export function MascotFigure(props: MascotFigureProps) {
  const manifest = useQuery(artManifestQueryOptions)
  const wanted = `mascot-${props.mood ?? 'base'}`
  const name = manifest.data?.[wanted] ? wanted : 'mascot-base'
  if (!manifest.data?.[name]) return null

  const beside = props.bubbleSide === 'left'
  const picture = (
    <ArtPicture
      name={name}
      alt=''
      sizes={props.sizes}
      className={props.imageClassName}
      imgClassName='object-contain'
    />
  )

  return (
    <figure
      className={cn(
        'flex items-center gap-2.5',
        beside ? 'flex-row' : 'flex-col',
        props.className
      )}
    >
      {/* The tail names its border colour: the page's default border colour
          reaches elements, not their ::after. */}
      {props.line && (
        <figcaption
          className={cn(
            'bg-background/95 font-mincho after:bg-background/95 after:border-border wt-bubble-pop relative max-w-44 rounded-2xl border px-3 py-1.5 text-xs leading-snug shadow-sm after:absolute after:size-3 after:rotate-45',
            beside
              ? 'origin-right text-left after:top-1/2 after:-right-[7px] after:-translate-y-1/2 after:border-t after:border-r'
              : 'origin-bottom text-center after:-bottom-[7px] after:left-1/2 after:-translate-x-1/2 after:border-r after:border-b'
          )}
        >
          {props.line}
        </figcaption>
      )}
      {props.pictureAction ? (
        <Button
          variant='ghost'
          aria-label={props.pictureAction.label}
          onClick={props.pictureAction.onClick}
          className='h-auto shrink-0 rounded-full p-0 hover:bg-transparent dark:hover:bg-transparent'
        >
          {picture}
        </Button>
      ) : (
        picture
      )}
    </figure>
  )
}
