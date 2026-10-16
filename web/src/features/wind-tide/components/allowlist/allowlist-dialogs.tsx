/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { AllowlistAddDialog } from './allowlist-add-dialog'
import { AllowlistNoteDialog } from './allowlist-note-dialog'
import { AllowlistStatusDialog } from './allowlist-status-dialog'
import { RejectRequestDialog } from './reject-request-dialog'

export function AllowlistDialogs() {
  return (
    <>
      <AllowlistAddDialog />
      <AllowlistNoteDialog />
      <AllowlistStatusDialog />
      <RejectRequestDialog />
    </>
  )
}
