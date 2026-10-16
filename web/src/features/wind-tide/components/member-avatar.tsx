/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { cn } from '@/lib/utils'

type MemberAvatarProps = {
  name: string
  /** The cached Discord avatar; empty for the default one. */
  avatarUrl: string
  /** Size and rings, such as `size-8`. */
  className?: string
  /** Size of the initial shown when there is no avatar, such as `text-xs`. */
  fallbackClassName?: string
}

/**
 * A Discord member's avatar as the server caches it (wind-tide/API.md §8).
 * Without a custom avatar, or while it fails to load, the member's initial
 * shows on a colour of their own.
 */
export function MemberAvatar(props: MemberAvatarProps) {
  return (
    <Avatar className={cn('size-8 shrink-0', props.className)}>
      {props.avatarUrl && <AvatarImage src={props.avatarUrl} alt='' />}
      <AvatarFallback
        className={cn('text-xs font-semibold', props.fallbackClassName)}
        style={getUserAvatarStyle(props.name)}
      >
        {getUserAvatarFallback(props.name)}
      </AvatarFallback>
    </Avatar>
  )
}
