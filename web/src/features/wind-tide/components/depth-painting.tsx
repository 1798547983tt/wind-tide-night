/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'

import { cn } from '@/lib/utils'

import {
  DEFAULT_PAINTING_SETTINGS,
  DepthPaintingRenderer,
  type PaintingSettings,
} from '../lib/depth-painting-gl'

type DepthPaintingProps = {
  imageUrl: string
  depthUrl: string
  /** Shown before WebGL is ready and wherever WebGL is unavailable. */
  fallbackSrc: string
  alt: string
  focus?: [number, number]
  zoom?: number
  parallax?: number
  focalDepth?: number
  night?: boolean
  rays?: number
  rayOrigin?: [number, number]
  horizon?: number
  moon?: [number, number]
  className?: string
  onReady?: () => void
}

const MAX_DPR = 1.75

/**
 * A full-bleed illustration with 2.5D depth parallax. Falls back to a plain
 * image when WebGL2 is missing, and holds still under reduced motion.
 */
export function DepthPainting(props: DepthPaintingProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const rendererRef = useRef<DepthPaintingRenderer | null>(null)
  const onReadyRef = useRef(props.onReady)
  const [ready, setReady] = useState(false)
  const [failed, setFailed] = useState(false)
  const reducedMotion = !!useReducedMotion()

  onReadyRef.current = props.onReady

  const settings: PaintingSettings = {
    focus: props.focus ?? DEFAULT_PAINTING_SETTINGS.focus,
    zoom: props.zoom ?? DEFAULT_PAINTING_SETTINGS.zoom,
    parallax: props.parallax ?? DEFAULT_PAINTING_SETTINGS.parallax,
    focalDepth: props.focalDepth ?? DEFAULT_PAINTING_SETTINGS.focalDepth,
    night: props.night ? 1 : 0,
    rays: props.rays ?? DEFAULT_PAINTING_SETTINGS.rays,
    rayOrigin: props.rayOrigin ?? DEFAULT_PAINTING_SETTINGS.rayOrigin,
    horizon: props.horizon ?? DEFAULT_PAINTING_SETTINGS.horizon,
    moon: props.moon ?? DEFAULT_PAINTING_SETTINGS.moon,
  }
  const settingsRef = useRef(settings)
  settingsRef.current = settings

  // Create the renderer once per image; settings flow in through setTarget.
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const renderer = DepthPaintingRenderer.create(
      canvas,
      settingsRef.current,
      !window.matchMedia('(prefers-reduced-motion: reduce)').matches
    )
    if (!renderer) {
      setFailed(true)
      onReadyRef.current?.()
      return
    }
    rendererRef.current = renderer
    let cancelled = false

    const resize = () => {
      const rect = canvas.getBoundingClientRect()
      renderer.resize(
        rect.width,
        rect.height,
        Math.min(window.devicePixelRatio || 1, MAX_DPR)
      )
    }
    resize()
    const resizeObserver = new ResizeObserver(resize)
    resizeObserver.observe(canvas)

    // Only spend frames while the painting is on screen and the tab is shown.
    let visible = false
    const sync = () => {
      if (visible && document.visibilityState === 'visible') renderer.start()
      else renderer.stop()
    }
    const intersection = new IntersectionObserver(([entry]) => {
      visible = !!entry?.isIntersecting
      sync()
    })
    intersection.observe(canvas)
    document.addEventListener('visibilitychange', sync)

    const onPointer = (event: PointerEvent) => {
      renderer.setPointer(
        (event.clientX / window.innerWidth) * 2 - 1,
        (event.clientY / window.innerHeight) * 2 - 1
      )
    }
    const onTilt = (event: DeviceOrientationEvent) => {
      if (event.gamma == null || event.beta == null) return
      renderer.setPointer(
        Math.max(-1, Math.min(1, event.gamma / 25)),
        Math.max(-1, Math.min(1, (event.beta - 45) / 25))
      )
    }
    window.addEventListener('pointermove', onPointer, { passive: true })
    window.addEventListener('deviceorientation', onTilt, { passive: true })

    renderer
      .load(props.imageUrl, props.depthUrl)
      .then(() => {
        if (cancelled) return
        setReady(true)
        onReadyRef.current?.()
      })
      .catch(() => {
        if (cancelled) return
        setFailed(true)
        onReadyRef.current?.()
      })

    return () => {
      cancelled = true
      resizeObserver.disconnect()
      intersection.disconnect()
      document.removeEventListener('visibilitychange', sync)
      window.removeEventListener('pointermove', onPointer)
      window.removeEventListener('deviceorientation', onTilt)
      renderer.destroy()
      rendererRef.current = null
    }
  }, [props.imageUrl, props.depthUrl])

  useEffect(() => {
    rendererRef.current?.setAnimate(!reducedMotion)
  }, [reducedMotion])

  const settingsKey = JSON.stringify(settings)
  useEffect(() => {
    rendererRef.current?.setTarget(settingsRef.current)
  }, [settingsKey])

  const focus = settings.focus
  return (
    <div className={cn('relative overflow-hidden', props.className)}>
      <img
        src={props.fallbackSrc}
        alt={props.alt}
        className={cn(
          'absolute inset-0 size-full object-cover transition-opacity duration-700',
          // Until the shader re-lights it, dim the day painting toward night.
          props.night && 'brightness-[0.45] saturate-[0.6]',
          ready && !failed && 'opacity-0'
        )}
        style={{ objectPosition: `${focus[0] * 100}% ${focus[1] * 100}%` }}
        decoding='async'
      />
      {!failed && (
        <canvas
          ref={canvasRef}
          aria-hidden='true'
          className={cn(
            'absolute inset-0 size-full transition-opacity duration-700',
            ready ? 'opacity-100' : 'opacity-0'
          )}
        />
      )}
    </div>
  )
}
