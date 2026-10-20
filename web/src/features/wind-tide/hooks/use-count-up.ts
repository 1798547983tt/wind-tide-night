/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useEffect, useRef, useState } from 'react'

type CountUpOptions = {
  /** How long a change takes to count through, in milliseconds. */
  duration?: number
  /** Show the value at once: for a member who prefers reduced motion. */
  disabled?: boolean
}

const DEFAULT_DURATION = 900

/**
 * A figure that counts from what it showed to the value it is given, easing
 * out over the duration. It starts on its first value without counting, and
 * a value that changes mid-count is counted to from wherever the count was.
 */
export function useCountUp(
  target: number,
  options: CountUpOptions = {}
): number {
  const duration = options.duration ?? DEFAULT_DURATION
  const disabled = options.disabled === true
  const [shown, setShown] = useState(target)
  // What is on screen, for a count that starts while another is running.
  const onScreen = useRef(target)

  useEffect(() => {
    const from = onScreen.current
    if (disabled || from === target) {
      onScreen.current = target
      setShown(target)
      return
    }
    let began = -1
    let frame = 0
    const step = (now: number) => {
      if (began < 0) began = now
      const progress = duration <= 0 ? 1 : Math.min(1, (now - began) / duration)
      const eased = 1 - (1 - progress) ** 3
      const value = Math.round(from + (target - from) * eased)
      onScreen.current = value
      setShown(value)
      if (progress < 1) frame = window.requestAnimationFrame(step)
    }
    frame = window.requestAnimationFrame(step)
    return () => window.cancelAnimationFrame(frame)
  }, [target, duration, disabled])

  if (disabled) return target
  return shown
}
