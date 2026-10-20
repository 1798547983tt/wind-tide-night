/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { useCountUp } from '../use-count-up'

type Options = NonNullable<Parameters<typeof useCountUp>[1]>
type Props = { target: number; options: Options }

// A clock and a frame queue of our own, so each frame runs when the test
// says and with the time the test says.
let now: number
let frames: Map<number, FrameRequestCallback>
let lastFrame: number

/** Runs every frame requested so far, ms later on the clock. */
function frame(ms: number) {
  now += ms
  const due = [...frames.values()]
  frames.clear()
  act(() => {
    for (const callback of due) callback(now)
  })
}

function count(target: number, options: Options = {}) {
  return renderHook((props: Props) => useCountUp(props.target, props.options), {
    initialProps: { target, options },
  })
}

beforeEach(() => {
  now = 1000
  frames = new Map()
  lastFrame = 0
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
    lastFrame += 1
    frames.set(lastFrame, callback)
    return lastFrame
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((id) => {
    frames.delete(id)
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('useCountUp', () => {
  test('starts on its first value without counting', () => {
    const { result } = count(40)
    expect(result.current).toBe(40)
    expect(frames.size).toBe(0)
  })

  test('counts through a change, easing out, and lands on the target', () => {
    const { result, rerender } = count(40)
    rerender({ target: 100, options: {} })
    expect(frames.size).toBe(1)
    frame(0)
    expect(result.current).toBe(40)
    // Half way through the time is 87.5% of the way through the count.
    frame(450)
    expect(result.current).toBe(93)
    frame(450)
    expect(result.current).toBe(100)
    expect(frames.size).toBe(0)
  })

  test('a new target mid-count is counted to from where the count was', () => {
    const { result, rerender } = count(40)
    rerender({ target: 100, options: {} })
    frame(0)
    frame(450)
    expect(result.current).toBe(93)
    rerender({ target: 0, options: {} })
    frame(0)
    expect(result.current).toBe(93)
    frame(900)
    expect(result.current).toBe(0)
    expect(frames.size).toBe(0)
  })

  test('a shorter duration counts faster', () => {
    const { result, rerender } = count(0, { duration: 100 })
    rerender({ target: 10, options: { duration: 100 } })
    frame(0)
    frame(100)
    expect(result.current).toBe(10)
  })

  test('disabled shows every value at once and asks for no frames', () => {
    const { result, rerender } = count(40, { disabled: true })
    rerender({ target: 100, options: { disabled: true } })
    expect(result.current).toBe(100)
    expect(frames.size).toBe(0)
  })

  test('unmounting cancels the frame still pending', () => {
    const { rerender, unmount } = count(40)
    rerender({ target: 100, options: {} })
    expect(frames.size).toBe(1)
    unmount()
    expect(frames.size).toBe(0)
  })
})
