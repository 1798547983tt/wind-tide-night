/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { create } from 'zustand'

import type { GateRefusal } from '@/features/wind-tide/types'

// The refusal stays in this tab's session storage so the gate page survives a
// reload, and stays out of the URL because it names the Discord account.
const GATE_KEY = 'wt.gate'
const GATE_STATES = new Set(['pending', 'rejected', 'removed'])

/** Whether a failed Discord sign-in is the admission gate turning it away. */
export function isGateRefusal(value: unknown): value is GateRefusal {
  if (!value || typeof value !== 'object') return false
  const refusal = value as Partial<GateRefusal>
  return (
    typeof refusal.wt_gate === 'string' &&
    GATE_STATES.has(refusal.wt_gate) &&
    typeof refusal.applicant?.discord_id === 'string' &&
    typeof refusal.requested_at === 'number' &&
    typeof refusal.retry_after === 'number'
  )
}

function readRefusal(): GateRefusal | null {
  try {
    const saved = JSON.parse(window.sessionStorage.getItem(GATE_KEY) ?? 'null')
    return isGateRefusal(saved) ? saved : null
  } catch {
    return null
  }
}

interface GateState {
  refusal: GateRefusal | null
  setRefusal: (refusal: GateRefusal) => void
}

export const useGateStore = create<GateState>()((set) => ({
  refusal: typeof window === 'undefined' ? null : readRefusal(),
  setRefusal: (refusal) => {
    try {
      window.sessionStorage.setItem(GATE_KEY, JSON.stringify(refusal))
    } catch {
      // Without storage the page still shows the refusal until a reload.
    }
    set({ refusal })
  },
}))
