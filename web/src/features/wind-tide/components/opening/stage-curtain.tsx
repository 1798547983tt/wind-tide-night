/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { cn } from '@/lib/utils'

type StageCurtainProps = {
  open: boolean
  /** Lifts the pelmet out of frame, for the camera push. */
  lift: boolean
  className?: string
}

/** Crimson velvet curtains that gather to the sides when opened. */
export function StageCurtain(props: StageCurtainProps) {
  return (
    <div
      aria-hidden='true'
      className={cn(
        'pointer-events-none absolute inset-0 overflow-hidden',
        props.className
      )}
    >
      <div
        className='wt-curtain-panel'
        data-side='left'
        data-open={props.open}
      />
      <div
        className='wt-curtain-panel'
        data-side='right'
        data-open={props.open}
      />
      <div className='wt-valance' data-lift={props.lift} />
    </div>
  )
}
