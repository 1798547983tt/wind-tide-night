/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { MemberAvatar } from '../member-avatar'

type MemberIdentityProps = {
  name: string
  username: string
  avatarUrl: string
}

/**
 * A Discord account as the owner recognizes it: avatar, name and @handle. A
 * table cell grows with its content, so the 15rem cap is what lets a long
 * name truncate at the column's width instead of crowding the other columns.
 */
export function MemberIdentity(props: MemberIdentityProps) {
  return (
    <div className='flex max-w-60 min-w-0 items-center gap-2.5'>
      <MemberAvatar name={props.name} avatarUrl={props.avatarUrl} />
      <div className='min-w-0'>
        <div className='truncate text-sm font-medium'>{props.name}</div>
        {props.username && props.username !== props.name && (
          <div className='text-muted-foreground truncate text-xs'>
            @{props.username}
          </div>
        )}
      </div>
    </div>
  )
}
