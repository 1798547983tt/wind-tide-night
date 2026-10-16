/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { cn } from '@/lib/utils'

type VerticalTitleProps = {
  text: string
  as?: 'h1' | 'h2' | 'p' | 'div'
  /** Milliseconds before the first character starts to appear. */
  delay?: number
  /** Milliseconds between characters. */
  stagger?: number
  /** While false the characters stay hidden, ready to be revealed. */
  play?: boolean
  className?: string
  charClassName?: string
}

/**
 * Top-to-bottom calligraphy (縦書き) whose characters soak in one by one
 * like brush strokes. Screen readers get the whole text at once.
 */
export function VerticalTitle(props: VerticalTitleProps) {
  const Tag = props.as ?? 'h1'
  const delay = props.delay ?? 0
  const stagger = props.stagger ?? 220
  const play = props.play ?? true

  return (
    <Tag
      aria-label={props.text}
      className={cn('wt-vertical font-brush leading-none', props.className)}
    >
      {[...props.text].map((char, index) => (
        <span
          // Characters can repeat; their position is the identity.
          // eslint-disable-next-line react/no-array-index-key
          key={index}
          aria-hidden='true'
          className={cn(
            play ? 'wt-ink-char' : 'inline-block opacity-0',
            props.charClassName
          )}
          style={
            play
              ? { animationDelay: `${delay + index * stagger}ms` }
              : undefined
          }
        >
          {char}
        </span>
      ))}
    </Tag>
  )
}
