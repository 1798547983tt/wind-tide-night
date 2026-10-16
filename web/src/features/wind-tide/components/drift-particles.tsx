/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useReducedMotion } from 'motion/react'
import { useEffect, useRef } from 'react'

import { cn } from '@/lib/utils'

/**
 * - petals: sakura petals tumbling on the wind (day).
 * - dust: warm motes rising slowly through a sunbeam (the opening).
 * - fireflies: tide-coloured glows wandering over the night sea.
 */
export type ParticleKind = 'petals' | 'dust' | 'fireflies'

type Particle = {
  x: number
  y: number
  vx: number
  vy: number
  size: number
  rotation: number
  spin: number
  phase: number
  rate: number
  sprite: number
}

type DriftParticlesProps = {
  kind: ParticleKind
  /** Multiplies the default amount; 1 suits a full-screen scene. */
  density?: number
  className?: string
}

const MAX_DPR = 2

function makeSprite(
  size: number,
  paint: (ctx: CanvasRenderingContext2D) => void
) {
  const canvas = document.createElement('canvas')
  canvas.width = size
  canvas.height = size
  const ctx = canvas.getContext('2d')
  if (ctx) {
    ctx.translate(size / 2, size / 2)
    paint(ctx)
  }
  return canvas
}

function petalSprites() {
  const tints = [
    ['#f6a9bf', '#fff1f5'],
    ['#f19bb5', '#ffe4ec'],
    ['#fbc6d4', '#ffffff'],
  ]
  return tints.map(([deep, light]) =>
    makeSprite(48, (ctx) => {
      const w = 20
      const h = 30
      const gradient = ctx.createLinearGradient(0, h / 2, 0, -h / 2)
      gradient.addColorStop(0, deep)
      gradient.addColorStop(1, light)
      ctx.fillStyle = gradient
      ctx.beginPath()
      ctx.moveTo(0, h * 0.5)
      ctx.bezierCurveTo(
        w * 0.62,
        h * 0.22,
        w * 0.58,
        -h * 0.34,
        w * 0.13,
        -h * 0.5
      )
      ctx.lineTo(0, -h * 0.36)
      ctx.lineTo(-w * 0.13, -h * 0.5)
      ctx.bezierCurveTo(-w * 0.58, -h * 0.34, -w * 0.62, h * 0.22, 0, h * 0.5)
      ctx.closePath()
      ctx.fill()
    })
  )
}

function glowSprite(inner: string, outer: string) {
  return makeSprite(64, (ctx) => {
    const gradient = ctx.createRadialGradient(0, 0, 0, 0, 0, 32)
    gradient.addColorStop(0, inner)
    gradient.addColorStop(0.25, outer)
    gradient.addColorStop(1, 'rgba(0,0,0,0)')
    ctx.fillStyle = gradient
    ctx.fillRect(-32, -32, 64, 64)
  })
}

function particleCount(kind: ParticleKind, area: number, density: number) {
  const perArea = { petals: 52_000, dust: 16_000, fireflies: 48_000 }[kind]
  const bounds = { petals: [6, 26], dust: [18, 90], fireflies: [8, 30] }[kind]
  const count = Math.round((area / perArea) * density)
  return Math.max(bounds[0], Math.min(bounds[1], count))
}

function spawn(
  kind: ParticleKind,
  width: number,
  height: number,
  initial: boolean
): Particle {
  const r = Math.random
  if (kind === 'petals') {
    // Wind blows from the left; new petals enter from the top or left edge.
    let x = r() * width * 0.9 - width * 0.1
    let y = -24
    if (initial) {
      x = r() * width
      y = r() * height
    } else if (r() < 0.35) {
      x = -20
      y = r() * height * 0.6
    }
    return {
      x,
      y,
      vx: 26 + r() * 46,
      vy: 28 + r() * 36,
      size: 0.45 + r() * 0.6,
      rotation: r() * Math.PI * 2,
      spin: (r() - 0.5) * 2.2,
      phase: r() * Math.PI * 2,
      rate: 1 + r() * 1.8,
      sprite: Math.floor(r() * 3),
    }
  }
  if (kind === 'dust') {
    return {
      x: r() * width,
      y: initial ? r() * height : height + 10,
      vx: (r() - 0.5) * 6,
      vy: -(3 + r() * 10),
      size: 0.08 + r() * 0.22,
      rotation: 0,
      spin: 0,
      phase: r() * Math.PI * 2,
      rate: 0.6 + r() * 1.6,
      sprite: 0,
    }
  }
  return {
    x: r() * width,
    y: height * (0.25 + r() * 0.75),
    vx: 0,
    vy: 0,
    size: 0.18 + r() * 0.3,
    rotation: r() * Math.PI * 2,
    spin: 8 + r() * 14,
    phase: r() * Math.PI * 2,
    rate: 0.5 + r() * 1.2,
    sprite: 0,
  }
}

/** Decorative particles on a transparent canvas; renders nothing under reduced motion. */
export function DriftParticles(props: DriftParticlesProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const reducedMotion = !!useReducedMotion()
  const density = props.density ?? 1

  useEffect(() => {
    const canvas = canvasRef.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx || reducedMotion) return

    const kind = props.kind
    let sprites: HTMLCanvasElement[] = []
    if (kind === 'petals') sprites = petalSprites()
    if (kind === 'dust') {
      sprites = [glowSprite('rgba(255,250,235,0.95)', 'rgba(255,226,170,0.35)')]
    }
    if (kind === 'fireflies') {
      sprites = [glowSprite('rgba(220,255,250,1)', 'rgba(111,217,209,0.45)')]
    }

    let width = 0
    let height = 0
    let particles: Particle[] = []
    const resize = () => {
      const rect = canvas.getBoundingClientRect()
      const dpr = Math.min(window.devicePixelRatio || 1, MAX_DPR)
      width = rect.width
      height = rect.height
      canvas.width = Math.round(width * dpr)
      canvas.height = Math.round(height * dpr)
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      const count = particleCount(kind, width * height, density)
      particles = Array.from({ length: count }, () =>
        spawn(kind, width, height, true)
      )
    }
    resize()
    const resizeObserver = new ResizeObserver(resize)
    resizeObserver.observe(canvas)

    let frame = 0
    let last = performance.now()
    let time = 0
    const tick = (now: number) => {
      const dt = Math.min((now - last) / 1000, 0.05)
      last = now
      time += dt
      ctx.clearRect(0, 0, width, height)
      ctx.globalCompositeOperation =
        kind === 'petals' ? 'source-over' : 'lighter'

      particles.forEach((p, index) => {
        if (kind === 'fireflies') {
          // Wander: heading turns smoothly, speed stays gentle.
          p.rotation += Math.sin(time * p.rate + p.phase) * 1.4 * dt
          p.vx = Math.cos(p.rotation) * p.spin
          p.vy = Math.sin(p.rotation) * p.spin * 0.6
        }
        const sway =
          kind === 'petals' ? Math.sin(time * p.rate + p.phase) * 22 : 0
        p.x += (p.vx + sway) * dt
        p.y += p.vy * dt
        p.rotation += kind === 'petals' ? p.spin * dt : 0

        const out =
          p.x > width + 40 || p.y > height + 40 || p.y < -60 || p.x < -60
        if (out && kind !== 'fireflies') {
          particles[index] = spawn(kind, width, height, false)
          return
        }
        if (kind === 'fireflies') {
          p.x = (p.x + width) % width
          p.y = Math.min(Math.max(p.y, height * 0.2), height)
        }

        const sprite = sprites[p.sprite]
        if (!sprite) return
        const flicker = 0.5 + 0.5 * Math.sin(time * p.rate * 2 + p.phase)
        ctx.save()
        ctx.translate(p.x, p.y)
        if (kind === 'petals') {
          ctx.rotate(p.rotation)
          // Tumbling: the petal turns edge-on and back.
          ctx.scale(p.size * Math.cos(time * p.rate + p.phase), p.size)
          ctx.globalAlpha = 0.9
        } else {
          ctx.scale(p.size, p.size)
          ctx.globalAlpha =
            kind === 'dust' ? 0.25 + 0.75 * flicker : 0.35 + 0.65 * flicker
        }
        ctx.drawImage(sprite, -sprite.width / 2, -sprite.height / 2)
        ctx.restore()
      })
      frame = requestAnimationFrame(tick)
    }

    let visible = false
    const sync = () => {
      const shouldRun = visible && document.visibilityState === 'visible'
      if (shouldRun && !frame) {
        last = performance.now()
        frame = requestAnimationFrame(tick)
      }
      if (!shouldRun && frame) {
        cancelAnimationFrame(frame)
        frame = 0
      }
    }
    const intersection = new IntersectionObserver(([entry]) => {
      visible = !!entry?.isIntersecting
      sync()
    })
    intersection.observe(canvas)
    document.addEventListener('visibilitychange', sync)

    return () => {
      cancelAnimationFrame(frame)
      resizeObserver.disconnect()
      intersection.disconnect()
      document.removeEventListener('visibilitychange', sync)
    }
  }, [props.kind, density, reducedMotion])

  if (reducedMotion) return null
  return (
    <canvas
      ref={canvasRef}
      aria-hidden='true'
      className={cn(
        'pointer-events-none absolute inset-0 size-full',
        props.className
      )}
    />
  )
}
