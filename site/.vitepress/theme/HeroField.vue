<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'

const canvasRef = ref<HTMLCanvasElement | null>(null)

let gl: WebGLRenderingContext | null = null
let program: WebGLProgram | null = null
let rafId = 0
let running = false
let reduced = false
let startTime = 0

const mouse = { x: 0.5, y: 0.5 }
const mouseTarget = { x: 0.5, y: 0.5 }

let uTime: WebGLUniformLocation | null = null
let uMouse: WebGLUniformLocation | null = null
let uRes: WebGLUniformLocation | null = null
let uLight: WebGLUniformLocation | null = null

let motionQuery: MediaQueryList | null = null
let visibilityObserver: IntersectionObserver | null = null
let offscreen = false

const VS = `
attribute vec2 a_pos;
void main(){ gl_Position = vec4(a_pos, 0.0, 1.0); }
`

// Petrol-blue recolor of a violet-nebula ambient field shader (R/B channel
// swap from the prior amber "rail signal" palette, matching brand.yaml's
// primary_color #107292 family instead of amber/gold).
const FS = `
precision highp float;
uniform vec2 u_res;
uniform float u_time;
uniform vec2 u_mouse;
uniform float u_light;

float hash(float n){ return fract(sin(n)*43758.5453123); }
float hash2(vec2 p){ return fract(sin(dot(p, vec2(127.1,311.7)))*43758.5453); }

float noise(float x){
    float i = floor(x);
    float f = fract(x);
    f = f*f*(3.0-2.0*f);
    return mix(hash(i), hash(i+1.0), f);
}

float fbm(float x, float octaves){
    float val = 0.0;
    float amp = 0.5;
    float freq = 1.0;
    for(int i = 0; i < 6; i++){
        if(float(i) >= octaves) break;
        val += amp * noise(x * freq);
        freq *= 2.17;
        amp *= 0.48;
    }
    return val;
}

float meteor(vec2 uv, float t){
    float cycle = mod(t * 0.15, 1.0);
    float seed = floor(t * 0.15);
    float h = hash(seed * 7.31);
    float h2 = hash(seed * 13.17);
    if(h > 0.30) return 0.0;
    vec2 start = vec2(0.2 + h2 * 0.6, 0.7 + h * 0.25);
    vec2 dir = normalize(vec2(1.0, -0.6 - h * 0.3));
    float progress = smoothstep(0.0, 0.7, cycle);
    vec2 pos = start + dir * progress * 0.5;
    vec2 toP = uv - pos;
    float along = dot(toP, dir);
    float perp = length(toP - dir * along);
    float trail = smoothstep(0.0, -0.12, along) * smoothstep(-0.18, -0.04, along);
    float core = smoothstep(0.003, 0.0, perp) * trail;
    float glow = smoothstep(0.012, 0.0, perp) * trail * 0.3;
    float fade = smoothstep(0.0, 0.1, cycle) * smoothstep(0.8, 0.55, cycle);
    return (core + glow) * fade;
}

float stars(vec2 uv, float density){
    vec2 cell = floor(uv * density);
    vec2 sub = fract(uv * density);
    float h = hash2(cell);
    float brightness = step(0.975, h);
    float size = 0.025 + h * 0.045;
    float d = length(sub - vec2(hash2(cell + 100.0), hash2(cell + 200.0)));
    float star = brightness * smoothstep(size, 0.0, d);
    // Floor at 0.35, not 0: stars twinkle (dim/brighten), never fully vanish.
    star *= 0.65 + 0.35 * sin(u_time * (1.0 + h * 3.0) + h * 6.28);
    return star;
}

void main(){
    vec2 uv = gl_FragCoord.xy / u_res;
    float aspect = u_res.x / u_res.y;
    vec2 mouse = u_mouse * 2.0 - 1.0;

    // Night palette (dark mode) mixed against a warm evening/sunset palette
    // (light mode) via u_light -- a flat pale-gray day sky read as lifeless,
    // so light mode is dusk instead of noon: warm orange sky with the same
    // petrol-blue rail-glow lines reading as lit signals against it.
    // Muted, not a literal painted sunset: a saturated orange landscape
    // read as decorative/whimsical rather than an infra tool. Desaturated
    // warm-neutral gradient keeps the mood without the postcard look.
    vec3 skyTop    = mix(vec3(0.010, 0.012, 0.020), vec3(0.975, 0.968, 0.958), u_light);
    vec3 skyMid    = mix(vec3(0.026, 0.021, 0.020), vec3(0.930, 0.895, 0.860), u_light);
    vec3 skyBottom = mix(vec3(0.024, 0.040, 0.058), vec3(0.860, 0.795, 0.740), u_light);

    float skyGrad = uv.y;
    vec3 col = mix(skyBottom, skyMid, smoothstep(0.3, 0.6, skyGrad));
    col = mix(col, skyTop, smoothstep(0.6, 1.0, skyGrad));

    float glowAmt = mix(1.0, 0.3, u_light);
    vec3 horizonGlowCol = mix(vec3(0.03, 0.15, 0.26), vec3(0.58, 0.40, 0.28), u_light);
    vec3 centerGlowCol  = mix(vec3(0.03, 0.12, 0.20), vec3(0.52, 0.36, 0.26), u_light);
    float horizonY = 0.35;
    float horizonGlow = exp(-pow((uv.y - horizonY) * 3.8, 2.0));
    col += horizonGlowCol * horizonGlow * 0.8 * glowAmt;

    float centerGlow = exp(-pow((uv.x - 0.5) * 1.5, 2.0)) * exp(-pow((uv.y - horizonY) * 4.0, 2.0));
    col += centerGlowCol * centerGlow * 0.6 * glowAmt;

    float starField = stars(uv * vec2(aspect, 1.0), 60.0)
                    + stars(uv * vec2(aspect, 1.0) + 500.0, 100.0) * 0.7
                    + stars(uv * vec2(aspect, 1.0) + 900.0, 160.0) * 0.4;

    float starMask = 1.0;
    float xC, yS, prof, mTop, mtn, rDist, rGlow, rAmb;
    vec3 lC = mix(vec3(0.045, 0.078, 0.11), vec3(0.680, 0.650, 0.620), u_light);

    // Layer 0 (farthest)
    xC = uv.x * aspect * 1.6 + u_time * 0.006 + mouse.x * 0.010;
    yS = mouse.y * 0.003;
    prof = fbm(xC, 5.0) * 0.10 + fbm(xC * 0.3 + 17.0, 3.0) * 0.07;
    mTop = 0.40 + prof + yS;
    mtn = smoothstep(mTop + 0.003, mTop - 0.001, uv.y);
    rDist = abs(uv.y - mTop);
    rGlow = smoothstep(0.012, 0.0, rDist) * 0.18;
    rAmb = smoothstep(0.04, 0.0, rDist) * 0.06;
    col = mix(col, lC, mtn);
    col += vec3(0.03, 0.16, 0.30) * rGlow * glowAmt;
    col += vec3(0.025, 0.10, 0.18) * rAmb * glowAmt;
    starMask *= (1.0 - mtn);

    // Layer 1
    xC = uv.x * aspect * 2.0 + u_time * 0.012 + mouse.x * 0.010;
    yS = mouse.y * 0.003;
    prof = fbm(xC, 5.0) * 0.10 + fbm(xC * 0.3 + 17.0, 3.0) * 0.07;
    mTop = 0.33 + prof + yS;
    mtn = smoothstep(mTop + 0.003, mTop - 0.001, uv.y);
    rDist = abs(uv.y - mTop);
    rGlow = smoothstep(0.012, 0.0, rDist) * 0.18;
    rAmb = smoothstep(0.04, 0.0, rDist) * 0.06;
    col = mix(col, lC, mtn);
    col += vec3(0.03, 0.16, 0.30) * rGlow * glowAmt;
    col += vec3(0.025, 0.10, 0.18) * rAmb * glowAmt;
    starMask *= (1.0 - mtn);

    // Layer 2
    xC = uv.x * aspect * 2.6 + u_time * 0.020 + mouse.x * 0.010;
    yS = mouse.y * 0.003;
    prof = fbm(xC, 5.0) * 0.10 + fbm(xC * 0.3 + 17.0, 3.0) * 0.07;
    mTop = 0.26 + prof + yS;
    mtn = smoothstep(mTop + 0.003, mTop - 0.001, uv.y);
    rDist = abs(uv.y - mTop);
    rGlow = smoothstep(0.012, 0.0, rDist) * 0.18;
    rAmb = smoothstep(0.04, 0.0, rDist) * 0.06;
    col = mix(col, lC, mtn);
    col += vec3(0.03, 0.16, 0.30) * rGlow * glowAmt;
    col += vec3(0.025, 0.10, 0.18) * rAmb * glowAmt;
    starMask *= (1.0 - mtn);

    // Layer 3
    xC = uv.x * aspect * 3.2 + u_time * 0.030 + mouse.x * 0.010;
    yS = mouse.y * 0.003;
    prof = fbm(xC, 5.0) * 0.10 + fbm(xC * 0.3 + 17.0, 3.0) * 0.07;
    mTop = 0.18 + prof + yS;
    mtn = smoothstep(mTop + 0.003, mTop - 0.001, uv.y);
    rDist = abs(uv.y - mTop);
    rGlow = smoothstep(0.012, 0.0, rDist) * 0.18;
    rAmb = smoothstep(0.04, 0.0, rDist) * 0.06;
    col = mix(col, lC, mtn);
    col += vec3(0.03, 0.16, 0.30) * rGlow * glowAmt;
    col += vec3(0.025, 0.10, 0.18) * rAmb * glowAmt;
    starMask *= (1.0 - mtn);

    // Layer 4 (nearest)
    xC = uv.x * aspect * 4.0 + u_time * 0.044 + mouse.x * 0.010;
    yS = mouse.y * 0.003;
    prof = fbm(xC, 5.0) * 0.10 + fbm(xC * 0.3 + 17.0, 3.0) * 0.07;
    mTop = 0.09 + prof + yS;
    mtn = smoothstep(mTop + 0.003, mTop - 0.001, uv.y);
    rDist = abs(uv.y - mTop);
    rGlow = smoothstep(0.012, 0.0, rDist) * 0.18;
    rAmb = smoothstep(0.04, 0.0, rDist) * 0.06;
    col = mix(col, lC, mtn);
    col += vec3(0.03, 0.16, 0.30) * rGlow * glowAmt;
    col += vec3(0.025, 0.10, 0.18) * rAmb * glowAmt;
    starMask *= (1.0 - mtn);

    float nightAmt = 1.0 - u_light;
    col += vec3(0.72, 0.90, 1.0) * starField * starMask * nightAmt;
    float met = meteor(uv * vec2(aspect, 1.0), u_time);
    col += vec3(0.32, 0.72, 1.0) * met * starMask * nightAmt;

    float vig = 1.0 - 0.3 * pow(length((uv - 0.5) * vec2(1.1, 1.6)), 2.0);
    col *= vig;

    col = pow(col, vec3(0.95));
    gl_FragColor = vec4(col, 1.0);
}
`

function compileShader(context: WebGLRenderingContext, type: number, source: string): WebGLShader | null {
  const shader = context.createShader(type)
  if (!shader) return null
  context.shaderSource(shader, source)
  context.compileShader(shader)
  if (!context.getShaderParameter(shader, context.COMPILE_STATUS)) {
    context.deleteShader(shader)
    return null
  }
  return shader
}

function resizeCanvas() {
  const canvas = canvasRef.value
  if (!canvas || !gl) return
  const dpr = Math.min(window.devicePixelRatio || 1, 1.5)
  canvas.width = Math.floor(window.innerWidth * dpr)
  canvas.height = Math.floor(window.innerHeight * dpr)
  gl.viewport(0, 0, canvas.width, canvas.height)
  gl.uniform2f(uRes, canvas.width, canvas.height)
}

function drawFrame(t: number) {
  if (!gl) return
  gl.uniform1f(uTime, t)
  gl.uniform2f(uMouse, mouse.x, mouse.y)
  // Always night: a light-mode sky palette was tried and read as a
  // decorative postcard rather than an infra tool, so the hero stays dark
  // regardless of the site's own light/dark toggle for now. u_light and
  // the day-palette mix() calls below are left in the shader rather than
  // ripped out, in case this gets revisited.
  gl.uniform1f(uLight, 0.0)
  gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)
}

function tick(time: number) {
  if (!startTime) startTime = time
  mouse.x += (mouseTarget.x - mouse.x) * 0.04
  mouse.y += (mouseTarget.y - mouse.y) * 0.04
  drawFrame((time - startTime) / 1000)
  rafId = requestAnimationFrame(tick)
}

function startLoop() {
  if (running || reduced || document.hidden || offscreen) return
  running = true
  rafId = requestAnimationFrame(tick)
}

function handleIntersect(entries: IntersectionObserverEntry[]) {
  offscreen = !entries[0]?.isIntersecting
  if (offscreen) {
    stopLoop()
  } else {
    startLoop()
  }
}

function stopLoop() {
  running = false
  if (rafId) cancelAnimationFrame(rafId)
  rafId = 0
}

function handleResize() {
  resizeCanvas()
  if (reduced) drawFrame(0)
}

function handleMouseMove(e: MouseEvent) {
  mouseTarget.x = e.clientX / window.innerWidth
  mouseTarget.y = 1 - e.clientY / window.innerHeight
}

function handleVisibilityChange() {
  if (document.hidden) {
    stopLoop()
  } else {
    startLoop()
  }
}

function handleMotionChange(e: MediaQueryListEvent) {
  reduced = e.matches
  if (reduced) {
    stopLoop()
    drawFrame(0)
  } else {
    startLoop()
  }
}

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return

  const context = canvas.getContext('webgl', { antialias: true, alpha: false, preserveDrawingBuffer: true })
  if (!context) return
  gl = context

  const vertexShader = compileShader(gl, gl.VERTEX_SHADER, VS)
  const fragmentShader = compileShader(gl, gl.FRAGMENT_SHADER, FS)
  if (!vertexShader || !fragmentShader) return

  const prog = gl.createProgram()
  if (!prog) return
  gl.attachShader(prog, vertexShader)
  gl.attachShader(prog, fragmentShader)
  gl.linkProgram(prog)
  if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) return
  gl.useProgram(prog)
  program = prog

  const quad = new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1])
  const buffer = gl.createBuffer()
  gl.bindBuffer(gl.ARRAY_BUFFER, buffer)
  gl.bufferData(gl.ARRAY_BUFFER, quad, gl.STATIC_DRAW)

  const posLoc = gl.getAttribLocation(prog, 'a_pos')
  gl.enableVertexAttribArray(posLoc)
  gl.vertexAttribPointer(posLoc, 2, gl.FLOAT, false, 0, 0)

  uTime = gl.getUniformLocation(prog, 'u_time')
  uMouse = gl.getUniformLocation(prog, 'u_mouse')
  uRes = gl.getUniformLocation(prog, 'u_res')
  uLight = gl.getUniformLocation(prog, 'u_light')

  resizeCanvas()

  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  reduced = motionQuery.matches
  motionQuery.addEventListener('change', handleMotionChange)

  window.addEventListener('resize', handleResize)
  window.addEventListener('mousemove', handleMouseMove, { passive: true })
  document.addEventListener('visibilitychange', handleVisibilityChange)

  // Scrolling the hero off-screen (anywhere past the fold) stops the loop
  // entirely instead of paying for 5 noise-layer fragment shader passes
  // the user can't see.
  visibilityObserver = new IntersectionObserver(handleIntersect, { threshold: 0 })
  visibilityObserver.observe(canvas)

  // startLoop() no-ops while document.hidden; paint frame 0 regardless.
  drawFrame(0)
  if (!reduced) startLoop()
})

onUnmounted(() => {
  stopLoop()
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('mousemove', handleMouseMove)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  motionQuery?.removeEventListener('change', handleMotionChange)
  visibilityObserver?.disconnect()

  if (gl) {
    gl.getExtension('WEBGL_lose_context')?.loseContext()
  }
  gl = null
  program = null
})
</script>

<template>
  <canvas ref="canvasRef" class="hero-field" aria-hidden="true"></canvas>
</template>

<style scoped>
.hero-field {
  /* Absolute, not fixed: contained by .VPHero (position: relative,
     overflow: hidden in custom.css) so the field scrolls away with the
     hero section instead of persisting as a page-wide backdrop. */
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  z-index: -1;
  pointer-events: none;
}
</style>
