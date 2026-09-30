<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'

type LineKind = 'command' | 'output' | 'success'
interface Line {
  kind: LineKind
  text: string
}

const script: Line[] = [
  { kind: 'command', text: 'curl -fsSL https://levelrail.com/install.sh | sudo sh' },
  { kind: 'output', text: 'checking host: docker, systemd, ports 80/443/8080 free' },
  { kind: 'output', text: 'installing control plane as a systemd service' },
  { kind: 'output', text: 'dashboard ready: https://198.51.100.42:8080 (setup token printed above)' },
  { kind: 'command', text: 'levelrail deploy myapp --image registry.example.com/acme/myapp:latest' },
  { kind: 'output', text: 'build pushed to the registry, readiness probe passed' },
  { kind: 'output', text: 'issuing TLS certificate' },
  { kind: 'success', text: 'myapp is live at https://myapp.example.com' },
]

const revealed = ref<{ kind: LineKind; text: string; done: boolean }[]>([])
let timers: ReturnType<typeof setTimeout>[] = []
let reduced = false
let motionQuery: MediaQueryList | null = null

function clearTimers() {
  timers.forEach(clearTimeout)
  timers = []
}

function showFinal() {
  clearTimers()
  revealed.value = script.map((line) => ({ ...line, done: true }))
}

function typeCommand(index: number, onDone: () => void) {
  const line = script[index]
  const slot = revealed.value.length
  revealed.value.push({ kind: line.kind, text: '', done: false })
  let chars = 0
  // Reassign the array slot (not mutate the pushed object in place) so
  // Vue's reactivity actually picks up each keystroke.
  const step = () => {
    chars += 1
    revealed.value.splice(slot, 1, {
      kind: line.kind,
      text: line.text.slice(0, chars),
      done: chars >= line.text.length,
    })
    if (chars < line.text.length) {
      timers.push(setTimeout(step, 22))
    } else {
      timers.push(setTimeout(onDone, 260))
    }
  }
  timers.push(setTimeout(step, 18))
}

function showOutput(index: number, onDone: () => void) {
  const line = script[index]
  timers.push(
    setTimeout(() => {
      revealed.value.push({ kind: line.kind, text: line.text, done: true })
      timers.push(setTimeout(onDone, line.kind === 'success' ? 0 : 180))
    }, 140),
  )
}

function playFrom(index: number) {
  if (index >= script.length) return
  const line = script[index]
  if (line.kind === 'command') {
    typeCommand(index, () => playFrom(index + 1))
  } else {
    showOutput(index, () => playFrom(index + 1))
  }
}

function handleMotionChange(e: MediaQueryListEvent) {
  reduced = e.matches
  if (reduced) showFinal()
}

onMounted(() => {
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  reduced = motionQuery.matches
  motionQuery.addEventListener('change', handleMotionChange)

  if (reduced) {
    showFinal()
  } else {
    playFrom(0)
  }
})

onUnmounted(() => {
  clearTimers()
  motionQuery?.removeEventListener('change', handleMotionChange)
})
</script>

<template>
  <div class="terminal-demo">
    <div class="terminal-demo__bar" aria-hidden="true">
      <span class="terminal-demo__title">install &rarr; deploy</span>
    </div>
    <div
      class="terminal-demo__body"
      role="img"
      aria-label="Terminal recording: installing Levelrail with the install script, then deploying an app that ends up live over HTTPS"
    >
      <div
        v-for="(line, i) in revealed"
        :key="i"
        class="terminal-demo__line"
        :class="`terminal-demo__line--${line.kind}`"
      >
        <template v-if="line.kind === 'command'"
          ><span class="terminal-demo__prompt">$</span> {{ line.text }}<span
            v-if="!line.done"
            class="terminal-demo__cursor"
          ></span
        ></template>
        <template v-else>{{ line.text }}</template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.terminal-demo {
  border: 1px solid var(--vp-c-divider);
  border-radius: 16px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.02), transparent 60%);
  overflow: hidden;
}

.terminal-demo__bar {
  padding: 10px 18px;
  border-bottom: 1px solid var(--vp-c-divider);
  background: var(--vp-c-bg-soft);
}

.terminal-demo__title {
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  letter-spacing: 0.04em;
  color: var(--vp-c-text-3);
}

.terminal-demo__body {
  min-height: 232px;
  padding: 20px 22px 24px;
  font-family: var(--vp-font-family-mono);
  font-size: 13.5px;
  line-height: 1.7;
  background: var(--vp-code-block-bg, var(--vp-c-bg-soft));
  color: var(--vp-c-text-2);
  overflow-x: auto;
}

.terminal-demo__line {
  white-space: pre-wrap;
  word-break: break-word;
}

.terminal-demo__line--command {
  color: var(--vp-c-text-1);
  font-weight: 500;
}

.terminal-demo__line--command:not(:first-child) {
  margin-top: 16px;
}

.terminal-demo__prompt {
  color: var(--vp-c-brand-1);
}

.terminal-demo__line--output {
  color: var(--vp-c-text-3);
}

.terminal-demo__line--success {
  margin-top: 4px;
  color: var(--vp-c-brand-1);
  font-weight: 600;
}

.terminal-demo__cursor {
  display: inline-block;
  width: 7px;
  height: 1em;
  margin-left: 2px;
  vertical-align: text-bottom;
  background: var(--vp-c-brand-1);
  animation: terminal-demo-blink 1s step-end infinite;
}

@media (prefers-reduced-motion: reduce) {
  .terminal-demo__cursor {
    animation: none;
    opacity: 0;
  }
}

@keyframes terminal-demo-blink {
  50% {
    opacity: 0;
  }
}
</style>
