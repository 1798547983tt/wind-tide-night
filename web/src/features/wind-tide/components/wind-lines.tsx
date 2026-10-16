/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { cn } from '@/lib/utils'

// Long, low S-curves like the speed lines anime draws for a breeze; the last
// one ends in a small curl.
const WIND_PATHS = [
  {
    d: 'M-60 230C180 190 330 270 530 220S870 160 1070 210 1390 270 1560 200',
    delay: 0,
    duration: 7.5,
  },
  {
    d: 'M-60 430C210 390 390 480 610 430S990 370 1210 420 1460 480 1560 440',
    delay: 2.1,
    duration: 8.5,
  },
  {
    d: 'M140 120C360 90 520 160 740 118S1090 70 1310 108',
    delay: 3.6,
    duration: 6.5,
  },
  {
    d: 'M-60 640C170 600 370 680 570 640S910 580 1130 630 1390 690 1560 650',
    delay: 1.2,
    duration: 9,
  },
  {
    d: 'M220 780C440 740 600 810 820 770S1120 720 1220 742C1300 760 1310 706 1262 700S1222 742 1268 756',
    delay: 4.8,
    duration: 8,
  },
]

type WindLinesProps = {
  className?: string
}

/** Decorative wind strokes; colour comes from `currentColor`. */
export function WindLines(props: WindLinesProps) {
  return (
    <svg
      aria-hidden='true'
      viewBox='0 0 1500 900'
      preserveAspectRatio='xMidYMid slice'
      className={cn(
        'pointer-events-none absolute inset-0 size-full',
        props.className
      )}
      fill='none'
    >
      {WIND_PATHS.map((path) => (
        <path
          key={path.d}
          d={path.d}
          pathLength={1}
          className='wt-wind-line'
          stroke='currentColor'
          strokeWidth={1.4}
          strokeLinecap='round'
          vectorEffect='non-scaling-stroke'
          style={{
            animationDelay: `${path.delay}s`,
            animationDuration: `${path.duration}s`,
          }}
        />
      ))}
    </svg>
  )
}
