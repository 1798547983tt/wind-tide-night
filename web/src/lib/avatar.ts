/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { CSSProperties } from 'react'

export type UserAvatarStyle = Pick<CSSProperties, 'backgroundColor' | 'color'>

function hashString(value: string): number {
  let hash = 0
  for (let i = 0; i < value.length; i++) {
    hash = (hash * 31 + value.charCodeAt(i)) >>> 0
  }
  return hash
}

// wind-tide: initials sit on the site's own muted traditional colours rather
// than any hue of the wheel, each dark enough for white initials to pass WCAG
// AA (4.5:1).
const AVATAR_COLORS = [
  '#b84d63', // rose
  '#2c7a80', // tide teal
  '#4b5c96', // dusk indigo
  '#80609f', // wisteria
  '#a2671c', // amber
  '#a8343f', // crimson
  '#3c7254', // pine
  '#36689c', // sea blue
  '#8a4569', // plum
  '#9a5a33', // bronze
] as const

export function getUserAvatarStyle(name: string): UserAvatarStyle {
  return {
    backgroundColor: AVATAR_COLORS[hashString(name) % AVATAR_COLORS.length],
    color: 'white',
  }
}

export function getUserAvatarFallback(name: string): string {
  return name.trim().charAt(0).toUpperCase() || '?'
}
