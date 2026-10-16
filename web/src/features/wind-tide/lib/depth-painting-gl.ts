/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/

/**
 * WebGL2 renderer for a "living painting": one illustration plus its depth
 * map (white = near), drawn as 2.5D parallax that follows the pointer and
 * drifts on its own. It can push in like a camera, add light shafts, and
 * re-light a day sky into a moonlit night sky with stars and a moon path.
 *
 * Every animated setting eases toward its target inside the frame loop, so
 * callers only ever declare where the painting should end up.
 */

export type PaintingSettings = {
  /** Image-space point (0–1, top-left origin) kept in frame when cropping. */
  focus: [number, number]
  /** 1 fills the canvas; larger values push in toward `focus`. */
  zoom: number
  /** Displacement strength in image UV units. */
  parallax: number
  /** Depth that stays still while everything else shifts (0 far, 1 near). */
  focalDepth: number
  /** 0 keeps the painting as drawn; 1 applies the full moonlit night. */
  night: number
  /** Strength of the light shafts, 0–1. */
  rays: number
  rayOrigin: [number, number]
  /** Image-space y of a sea horizon for the night grade, or -1 for none. */
  horizon: number
  /**
   * Screen-space point (0–1, top-left origin) where the night moon hangs, so
   * callers can keep it clear of the page's own text at every screen size.
   */
  moon: [number, number]
}

export const DEFAULT_PAINTING_SETTINGS: PaintingSettings = {
  focus: [0.5, 0.5],
  zoom: 1,
  parallax: 0.018,
  focalDepth: 0.6,
  night: 0,
  rays: 0,
  rayOrigin: [0.15, -0.05],
  horizon: -1,
  moon: [0.1, 0.2],
}

// Seconds for each eased setting to cover ~63% of the remaining distance.
const EASE_TIME = { zoom: 0.55, focus: 0.6, night: 0.45, pointer: 0.35 }

const VERTEX_SHADER = `#version 300 es
in vec2 aPos;
out vec2 vUv;
void main() {
  vUv = vec2(aPos.x * 0.5 + 0.5, 0.5 - aPos.y * 0.5);
  gl_Position = vec4(aPos, 0.0, 1.0);
}`

const FRAGMENT_SHADER = `#version 300 es
precision highp float;
in vec2 vUv;
out vec4 outColor;

uniform sampler2D uImage;
uniform sampler2D uDepth;
uniform vec2 uResolution;
uniform vec2 uImageSize;
uniform vec2 uFocus;
uniform float uZoom;
uniform vec2 uPointer;
uniform float uParallax;
uniform float uFocalDepth;
uniform float uTime;
uniform float uNight;
uniform float uRays;
uniform vec2 uRayOrigin;
uniform float uHorizon;
uniform vec2 uMoon;
uniform float uDpr;

float hash(vec2 p) {
  return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453123);
}

float depthAt(vec2 uv) {
  return texture(uDepth, uv).r;
}

vec2 coverUv(vec2 uv) {
  float canvasAspect = uResolution.x / uResolution.y;
  float imageAspect = uImageSize.x / uImageSize.y;
  vec2 span = vec2(1.0);
  if (canvasAspect > imageAspect) {
    span.y = imageAspect / canvasAspect;
  } else {
    span.x = canvasAspect / imageAspect;
  }
  // Leave room at the edges for the parallax shift.
  span *= (1.0 - 2.4 * uParallax) / uZoom;
  vec2 halfSpan = span * 0.5;
  vec2 center = clamp(uFocus, halfSpan, 1.0 - halfSpan);
  return center + (uv - 0.5) * span;
}

vec3 nightColor(vec3 c, vec2 uv, float d) {
  float aspect = uImageSize.x / uImageSize.y;
  float near = smoothstep(0.16, 0.5, d);
  float lum = dot(c, vec3(0.299, 0.587, 0.114));
  // The moon is placed on screen but drawn into the sky, so it rides with it.
  vec2 moon = coverUv(uMoon);

  // Sky: midnight at the top easing to indigo near the horizon.
  float h = uHorizon > 0.0 ? clamp(uv.y / uHorizon, 0.0, 1.0) : uv.y;
  vec3 sky = mix(vec3(0.018, 0.03, 0.1), vec3(0.11, 0.13, 0.32), smoothstep(0.0, 1.0, h));
  // Bright patches low in the sky are clouds; the pale haze higher up is not.
  float cloud = smoothstep(0.8, 0.98, lum) * smoothstep(0.35, 0.6, h);
  sky = mix(sky, vec3(0.6, 0.66, 0.86), cloud * 0.5);

  // A crescent like the site mark: a disc with an offset disc taken out.
  vec2 m = (uv - moon) * vec2(aspect, 1.0);
  float r = length(m);
  float disc = smoothstep(0.046, 0.0425, r);
  float shade = smoothstep(0.0368, 0.0333, length(m - vec2(0.0244, -0.0202)));
  sky += vec3(1.0, 0.95, 0.84) * disc * (1.0 - shade);
  sky += vec3(0.42, 0.5, 0.8) * disc * shade * 0.08;
  sky += vec3(0.48, 0.58, 0.95) * exp(-r * 8.0) * 0.32;

  // Stars live in screen space so they stay pin-sharp.
  vec2 sp = gl_FragCoord.xy / (22.0 * uDpr);
  vec2 cell = floor(sp);
  float hs = hash(cell);
  vec2 jitter = vec2(hash(cell + 3.1), hash(cell + 7.7)) - 0.5;
  float star = smoothstep(0.09, 0.0, length(fract(sp) - 0.5 - jitter * 0.6));
  star *= step(0.82, hs) * (0.55 + 0.45 * sin(uTime * (0.8 + hs * 2.4) + hs * 50.0));
  sky += vec3(0.85, 0.9, 1.0) * star * (1.0 - cloud) * (1.0 - disc) * (1.0 - smoothstep(0.55, 0.95, h));

  // Sea: deep navy with the original wave texture and a glittering moon path.
  float seaMask = uHorizon > 0.0 ? smoothstep(uHorizon - 0.003, uHorizon + 0.003, uv.y) : 0.0;
  vec3 sea = mix(vec3(0.05, 0.08, 0.2), vec3(0.01, 0.02, 0.07), clamp((uv.y - uHorizon) * 6.0, 0.0, 1.0));
  sea += vec3(0.2, 0.3, 0.55) * lum * 0.22;
  float band = exp(-pow((uv.x - moon.x) * 7.0, 2.0));
  float glint = step(0.9, hash(floor(gl_FragCoord.xy / (2.0 * uDpr)) + floor(uTime * 5.0)));
  sea += vec3(0.75, 0.86, 1.0) * glint * band * 0.9;
  vec3 far = mix(sky, sea, seaMask);

  // Subject: dimmed into cool moonlight but keeping part of its own colour,
  // lit from above so it settles into shade lower down, with a rim where it
  // meets the sky on the moon side.
  vec3 moonlit = mix(vec3(lum), c, 0.55) * vec3(0.34, 0.4, 0.7);
  vec3 subject = pow(moonlit, vec3(1.3)) * mix(1.25, 0.8, smoothstep(0.1, 0.95, uv.y));
  vec2 toMoon = normalize((moon - uv) * vec2(aspect, 1.0));
  float moonSide = depthAt(uv + toMoon * vec2(1.0 / aspect, 1.0) * 0.012);
  float rim = clamp((d - moonSide) * 3.5, 0.0, 1.0);
  subject += vec3(0.6, 0.72, 1.0) * rim * 0.45;

  return mix(far, subject, near);
}

void main() {
  vec2 base = coverUv(vUv);
  vec2 shift = uPointer * uParallax;
  float d = depthAt(base);
  vec2 uv = base + shift * (d - uFocalDepth);
  d = depthAt(uv);
  uv = clamp(base + shift * (d - uFocalDepth), 0.0, 1.0);
  d = depthAt(uv);
  vec3 color = texture(uImage, uv).rgb;

  if (uRays > 0.0) {
    vec2 rd = (uv - uRayOrigin) * vec2(uImageSize.x / uImageSize.y, 1.0);
    float ang = atan(rd.y, rd.x);
    float fan = 0.5 + 0.5 * sin(ang * 26.0 + uTime * 0.35) * sin(ang * 11.0 - uTime * 0.23);
    float shaft = pow(fan, 2.5) * exp(-length(rd) * 1.35) * (1.0 - 0.6 * d);
    color += vec3(1.0, 0.92, 0.78) * shaft * 0.22 * uRays;
  }

  if (uNight > 0.001) {
    color = mix(color, nightColor(color, uv, d), uNight);
  }

  vec2 q = vUv - 0.5;
  color *= mix(0.8, 1.0, smoothstep(0.95, 0.25, length(q * vec2(1.0, 1.15))));
  outColor = vec4(color, 1.0);
}`

const UNIFORMS = [
  'uImage',
  'uDepth',
  'uResolution',
  'uImageSize',
  'uFocus',
  'uZoom',
  'uPointer',
  'uParallax',
  'uFocalDepth',
  'uTime',
  'uNight',
  'uRays',
  'uRayOrigin',
  'uHorizon',
  'uMoon',
  'uDpr',
] as const

type UniformName = (typeof UNIFORMS)[number]

function compile(gl: WebGL2RenderingContext, type: number, source: string) {
  const shader = gl.createShader(type)
  if (!shader) return null
  gl.shaderSource(shader, source)
  gl.compileShader(shader)
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    gl.deleteShader(shader)
    return null
  }
  return shader
}

async function loadImage(url: string): Promise<HTMLImageElement> {
  const image = new Image()
  image.decoding = 'async'
  image.src = url
  await image.decode()
  return image
}

function ease(current: number, target: number, dt: number, tau: number) {
  return current + (target - current) * (1 - Math.exp(-dt / tau))
}

export class DepthPaintingRenderer {
  private readonly gl: WebGL2RenderingContext
  private readonly program: WebGLProgram
  private readonly uniforms: Record<UniformName, WebGLUniformLocation | null>
  private readonly textures: WebGLTexture[] = []
  private readonly buffer: WebGLBuffer | null
  private imageSize: [number, number] = [1, 1]
  private target: PaintingSettings
  private current: PaintingSettings
  private pointerTarget: [number, number] = [0, 0]
  private pointer: [number, number] = [0, 0]
  private size: [number, number] = [1, 1]
  private dpr = 1
  private frame = 0
  private lastTime = 0
  private elapsed = 0
  private loaded = false
  private destroyed = false
  private animate: boolean

  private constructor(
    gl: WebGL2RenderingContext,
    program: WebGLProgram,
    settings: PaintingSettings,
    animate: boolean
  ) {
    this.gl = gl
    this.program = program
    this.target = { ...settings }
    this.current = { ...settings }
    this.animate = animate
    this.uniforms = Object.fromEntries(
      UNIFORMS.map((name) => [name, gl.getUniformLocation(program, name)])
    ) as Record<UniformName, WebGLUniformLocation | null>

    const buffer = gl.createBuffer()
    this.buffer = buffer
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer)
    gl.bufferData(
      gl.ARRAY_BUFFER,
      new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]),
      gl.STATIC_DRAW
    )
    const position = gl.getAttribLocation(program, 'aPos')
    gl.enableVertexAttribArray(position)
    gl.vertexAttribPointer(position, 2, gl.FLOAT, false, 0, 0)
  }

  /** Returns null when WebGL2 or the shaders are unavailable. */
  static create(
    canvas: HTMLCanvasElement,
    settings: PaintingSettings,
    animate: boolean
  ): DepthPaintingRenderer | null {
    const gl = canvas.getContext('webgl2', {
      antialias: false,
      alpha: false,
      premultipliedAlpha: false,
      powerPreference: 'high-performance',
    })
    if (!gl) return null
    const vertex = compile(gl, gl.VERTEX_SHADER, VERTEX_SHADER)
    const fragment = compile(gl, gl.FRAGMENT_SHADER, FRAGMENT_SHADER)
    const program = gl.createProgram()
    if (!vertex || !fragment || !program) return null
    gl.attachShader(program, vertex)
    gl.attachShader(program, fragment)
    gl.linkProgram(program)
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) return null
    gl.useProgram(program)
    return new DepthPaintingRenderer(gl, program, settings, animate)
  }

  async load(imageUrl: string, depthUrl: string): Promise<void> {
    const [image, depth] = await Promise.all([
      loadImage(imageUrl),
      loadImage(depthUrl),
    ])
    // A canvas keeps one WebGL context, so a renderer replaced while its
    // images were loading (a remount, a new image) must not touch it again.
    if (this.destroyed) return
    const gl = this.gl
    gl.useProgram(this.program)
    ;[image, depth].forEach((source, unit) => {
      const texture = gl.createTexture()
      if (!texture) return
      this.textures.push(texture)
      gl.activeTexture(gl.TEXTURE0 + unit)
      gl.bindTexture(gl.TEXTURE_2D, texture)
      gl.texImage2D(
        gl.TEXTURE_2D,
        0,
        gl.RGBA,
        gl.RGBA,
        gl.UNSIGNED_BYTE,
        source
      )
      gl.generateMipmap(gl.TEXTURE_2D)
      gl.texParameteri(
        gl.TEXTURE_2D,
        gl.TEXTURE_MIN_FILTER,
        gl.LINEAR_MIPMAP_LINEAR
      )
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
    })
    gl.uniform1i(this.uniforms.uImage, 0)
    gl.uniform1i(this.uniforms.uDepth, 1)
    this.imageSize = [image.naturalWidth, image.naturalHeight]
    this.loaded = true
    this.draw()
  }

  setTarget(settings: Partial<PaintingSettings>, immediate = false) {
    this.target = { ...this.target, ...settings }
    if (immediate || !this.animate) {
      this.current = { ...this.target }
    }
    // Without the frame loop running, still show the new state.
    if (!this.frame) this.draw()
  }

  /** Pointer position in -1…1 on both axes, 0 being the centre. */
  setPointer(x: number, y: number) {
    this.pointerTarget = [x, y]
  }

  setAnimate(animate: boolean) {
    this.animate = animate
    if (!animate) {
      this.stop()
      this.current = { ...this.target }
      this.pointer = [0, 0]
      this.draw()
    }
  }

  resize(width: number, height: number, dpr: number) {
    const canvas = this.gl.canvas as HTMLCanvasElement
    this.size = [Math.max(1, width), Math.max(1, height)]
    this.dpr = dpr
    canvas.width = Math.round(this.size[0] * dpr)
    canvas.height = Math.round(this.size[1] * dpr)
    this.gl.viewport(0, 0, canvas.width, canvas.height)
    this.draw()
  }

  start() {
    if (this.frame || !this.animate || this.destroyed) return
    this.lastTime = performance.now()
    const tick = (now: number) => {
      const dt = Math.min((now - this.lastTime) / 1000, 0.05)
      this.lastTime = now
      this.step(dt)
      this.draw()
      this.frame = requestAnimationFrame(tick)
    }
    this.frame = requestAnimationFrame(tick)
  }

  stop() {
    if (!this.frame) return
    cancelAnimationFrame(this.frame)
    this.frame = 0
  }

  destroy() {
    this.stop()
    this.destroyed = true
    const gl = this.gl
    this.textures.forEach((texture) => gl.deleteTexture(texture))
    gl.deleteBuffer(this.buffer)
    gl.deleteProgram(this.program)
  }

  private step(dt: number) {
    this.elapsed += dt
    const t = this.elapsed
    const c = this.current
    const target = this.target
    c.zoom = ease(c.zoom, target.zoom, dt, EASE_TIME.zoom)
    c.night = ease(c.night, target.night, dt, EASE_TIME.night)
    c.focus = [
      ease(c.focus[0], target.focus[0], dt, EASE_TIME.focus),
      ease(c.focus[1], target.focus[1], dt, EASE_TIME.focus),
    ]
    c.parallax = target.parallax
    c.focalDepth = target.focalDepth
    c.rays = target.rays
    c.rayOrigin = target.rayOrigin
    c.horizon = target.horizon
    c.moon = target.moon

    // The painting keeps breathing even when nobody moves the pointer.
    const breathX = Math.sin(t * 0.23) * 0.3 + Math.sin(t * 0.11) * 0.12
    const breathY = Math.cos(t * 0.19) * 0.2
    this.pointer = [
      ease(
        this.pointer[0],
        this.pointerTarget[0] + breathX,
        dt,
        EASE_TIME.pointer
      ),
      ease(
        this.pointer[1],
        this.pointerTarget[1] + breathY,
        dt,
        EASE_TIME.pointer
      ),
    ]
  }

  private draw() {
    if (!this.loaded || this.destroyed) return
    const gl = this.gl
    const u = this.uniforms
    const c = this.current
    gl.useProgram(this.program)
    gl.uniform2f(
      u.uResolution,
      this.size[0] * this.dpr,
      this.size[1] * this.dpr
    )
    gl.uniform2f(u.uImageSize, this.imageSize[0], this.imageSize[1])
    gl.uniform2f(u.uFocus, c.focus[0], c.focus[1])
    gl.uniform1f(u.uZoom, c.zoom)
    gl.uniform2f(u.uPointer, this.pointer[0], this.pointer[1])
    gl.uniform1f(u.uParallax, c.parallax)
    gl.uniform1f(u.uFocalDepth, c.focalDepth)
    gl.uniform1f(u.uTime, this.elapsed)
    gl.uniform1f(u.uNight, c.night)
    gl.uniform1f(u.uRays, c.rays)
    gl.uniform2f(u.uRayOrigin, c.rayOrigin[0], c.rayOrigin[1])
    gl.uniform1f(u.uHorizon, c.horizon)
    gl.uniform2f(u.uMoon, c.moon[0], c.moon[1])
    gl.uniform1f(u.uDpr, this.dpr)
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)
  }
}
