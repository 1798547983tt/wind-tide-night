/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useEffect, useState } from 'react'

/** Unix seconds, ticking once a second while mounted; for countdowns. */
export function useNowSeconds() {
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000))
  useEffect(() => {
    const timer = window.setInterval(
      () => setNow(Math.floor(Date.now() / 1000)),
      1000
    )
    return () => window.clearInterval(timer)
  }, [])
  return now
}
