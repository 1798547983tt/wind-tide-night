/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { MascotFigure } from './mascot-figure'

type ErrorMascotProps = {
  /** Her own line about what went wrong, above the page's plain words. */
  line: string
  /** In tears by default; asleep while the site is down for maintenance. */
  mood?: 'cry' | 'sleep'
}

/**
 * The mascot on the error pages, phones included (wind-tide/DESIGN.md §8).
 * Like everywhere else she stays away while no mascot art is uploaded, and
 * the page explains itself without her.
 */
export function ErrorMascot(props: ErrorMascotProps) {
  return (
    <MascotFigure
      mood={props.mood ?? 'cry'}
      line={props.line}
      sizes='128px'
      className='mb-2'
      imageClassName='size-28 sm:size-32'
    />
  )
}
