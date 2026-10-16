/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import React, { useMemo, useState } from 'react'

import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'

import type { AllowlistEntry, EntryRequest } from '../../types'

export type AllowlistDialog =
  | 'add'
  | 'edit-note'
  | 'remove'
  | 'restore'
  | 'reject'

type AllowlistContextValue = {
  open: AllowlistDialog | null
  setOpen: (dialog: AllowlistDialog | null) => void
  currentEntry: AllowlistEntry | null
  setCurrentEntry: (entry: AllowlistEntry | null) => void
  currentRequest: EntryRequest | null
  setCurrentRequest: (request: EntryRequest | null) => void
  /** Step-up ceremony for removing or restoring an entry with an account. */
  requestVerification: ReturnType<
    typeof useSecureVerification
  >['requestVerification']
  /** True while the step-up dialog is showing; confirm dialogs yield to it. */
  verificationActive: boolean
}

const AllowlistContext = React.createContext<AllowlistContextValue | null>(null)

export function AllowlistProvider(props: { children: React.ReactNode }) {
  const [open, setOpen] = useState<AllowlistDialog | null>(null)
  const [currentEntry, setCurrentEntry] = useState<AllowlistEntry | null>(null)
  const [currentRequest, setCurrentRequest] = useState<EntryRequest | null>(
    null
  )
  // Mounted once here, like the users page; consumers only get the proof.
  const verification = useSecureVerification()
  const requestVerification = verification.requestVerification
  const verificationActive = verification.isActive

  const value = useMemo(
    () => ({
      open,
      setOpen,
      currentEntry,
      setCurrentEntry,
      currentRequest,
      setCurrentRequest,
      requestVerification,
      verificationActive,
    }),
    [
      open,
      currentEntry,
      currentRequest,
      requestVerification,
      verificationActive,
    ]
  )

  return (
    <AllowlistContext value={value}>
      {props.children}
      <SecureVerificationDialog {...verification.dialogProps} />
    </AllowlistContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAllowlist() {
  const context = React.useContext(AllowlistContext)
  if (!context) {
    throw new Error('useAllowlist has to be used within <AllowlistProvider>')
  }
  return context
}
