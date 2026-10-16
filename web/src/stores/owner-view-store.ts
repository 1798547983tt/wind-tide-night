/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { create } from 'zustand'

// The owner can browse the console the way members see it (wind-tide/
// DESIGN.md §3.1). Only the interface changes: the account, its data and its
// permissions stay the owner's. The choice is remembered in this browser and
// starts on the admin view.
const VIEW_KEY = 'wt.owner.view'

export type OwnerView = 'admin' | 'member'

function readOwnerView(): OwnerView {
  try {
    return window.localStorage.getItem(VIEW_KEY) === 'member'
      ? 'member'
      : 'admin'
  } catch {
    return 'admin'
  }
}

type OwnerViewState = {
  view: OwnerView
  setView: (view: OwnerView) => void
}

export const useOwnerViewStore = create<OwnerViewState>()((set) => ({
  view: typeof window === 'undefined' ? 'admin' : readOwnerView(),
  setView: (view) => {
    try {
      window.localStorage.setItem(VIEW_KEY, view)
    } catch {
      // Without storage the choice lasts until the page reloads.
    }
    set({ view })
  },
}))
