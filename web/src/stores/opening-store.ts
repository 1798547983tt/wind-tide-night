/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { create } from 'zustand'

// The opening plays once per browser session; the footer can replay it.
const SEEN_KEY = 'wt.opening.seen'

function hasSeenOpening(): boolean {
  try {
    return window.sessionStorage.getItem(SEEN_KEY) === '1'
  } catch {
    return false
  }
}

function markOpeningSeen() {
  try {
    window.sessionStorage.setItem(SEEN_KEY, '1')
  } catch {
    // Storage can be unavailable (private mode); the opening just plays again.
  }
}

/**
 * - playing: the theatre covers the page.
 * - washing: the tide is sweeping the theatre away; the page below starts
 *   its own entrance.
 * - done: nothing is covering the page.
 */
export type OpeningStage = 'playing' | 'washing' | 'done'

type OpeningState = {
  stage: OpeningStage
  replay: () => void
  wash: () => void
  finish: () => void
}

export const useOpeningStore = create<OpeningState>()((set) => ({
  stage:
    typeof window !== 'undefined' && !hasSeenOpening() ? 'playing' : 'done',
  replay: () => set({ stage: 'playing' }),
  wash: () => set({ stage: 'washing' }),
  finish: () => {
    markOpeningSeen()
    set({ stage: 'done' })
  },
}))
