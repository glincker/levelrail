<script setup lang="ts">
import { ref } from 'vue'

type LineKind = 'command' | 'output' | 'condition'
interface LogLine {
  kind: LineKind
  text: string
}
interface Step {
  index: string
  title: string
  summary: string
  lines: LogLine[]
}

const steps: Step[] = [
  {
    index: '01',
    title: 'Push to your git repo',
    summary: 'GitHub, GitLab, or Bitbucket webhooks fire on every push, with the commit SHA tracked end to end.',
    lines: [
      { kind: 'command', text: 'git push origin main' },
      { kind: 'output', text: 'webhook received: commit a3f91c2 on main' },
      { kind: 'output', text: 'queuing deploy for app web' },
    ],
  },
  {
    index: '02',
    title: 'Build through BuildKit',
    summary: 'A Dockerfile, a Compose file, or Railpack auto-detection builds through BuildKit, with remote cache and live log streaming.',
    lines: [
      { kind: 'output', text: 'building web (Dockerfile) via BuildKit' },
      { kind: 'output', text: '#6 [3/4] RUN go build -o /bin/web ./cmd/web' },
      { kind: 'output', text: '#6 DONE 11.2s' },
      { kind: 'output', text: '#9 exporting to image' },
      { kind: 'condition', text: 'build complete: web:a3f91c2 (18.4s)' },
    ],
  },
  {
    index: '03',
    title: 'Gate on a real health check',
    summary: 'The new container only becomes a cutover candidate once its readiness probe passes, not once the process merely starts.',
    lines: [
      { kind: 'output', text: 'starting web:a3f91c2' },
      { kind: 'output', text: 'GET /healthz -> connection refused' },
      { kind: 'output', text: 'GET /healthz -> 200 OK (1/2 consecutive)' },
      { kind: 'output', text: 'GET /healthz -> 200 OK (2/2 consecutive)' },
      { kind: 'condition', text: 'readiness probe passed' },
    ],
  },
  {
    index: '04',
    title: 'Cut over traffic',
    summary: 'Rolling or blue-green strategy: route traffic to the new container, drain the old one, and keep the prior image pinned for rollback.',
    lines: [
      { kind: 'command', text: 'levelrail deploy web' },
      { kind: 'output', text: 'strategy: rolling' },
      { kind: 'output', text: 'routing 100% of traffic to web:a3f91c2' },
      { kind: 'output', text: 'draining web:f81c0ad (30s grace period)' },
      { kind: 'condition', text: 'condition: Available=True reason=RolloutComplete' },
    ],
  },
]

const activeIndex = ref(0)
</script>

<template>
  <div class="flow">
    <div
      v-for="(step, i) in steps"
      :key="step.index"
      class="flow-step"
      :class="{ 'flow-step--active': activeIndex === i }"
    >
      <button
        type="button"
        class="flow-step__header"
        :aria-expanded="activeIndex === i"
        :aria-controls="`flow-panel-${step.index}`"
        @click="activeIndex = i"
      >
        <span class="flow-step__index">{{ step.index }}</span>
        <span class="flow-step__heading">
          <span class="flow-step__title">{{ step.title }}</span>
          <span class="flow-step__summary">{{ step.summary }}</span>
        </span>
      </button>
      <div
        v-show="activeIndex === i"
        :id="`flow-panel-${step.index}`"
        class="flow-step__panel"
        role="region"
      >
        <div class="flow-step__terminal" role="img" :aria-label="`Log output for: ${step.title}`">
          <div
            v-for="(line, li) in step.lines"
            :key="li"
            class="flow-step__line"
            :class="`flow-step__line--${line.kind}`"
          >
            <span v-if="line.kind === 'command'" class="flow-step__prompt">$</span>{{ ' ' }}{{ line.text }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.flow {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 24px;
}

.flow-step {
  border: 1px solid var(--vp-c-divider);
  border-radius: 16px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.02), transparent 60%);
  overflow: hidden;
  transition: border-color 0.25s cubic-bezier(0.32, 0.72, 0, 1);
}

.flow-step--active {
  border-color: var(--vp-c-brand-1);
}

.flow-step__header {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  width: 100%;
  padding: 18px 22px;
  border: none;
  background: transparent;
  text-align: left;
  font: inherit;
  color: inherit;
  cursor: pointer;
}

.flow-step__index {
  flex-shrink: 0;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  font-weight: 600;
  padding-top: 2px;
  color: var(--vp-c-text-3);
  transition: color 0.25s ease;
}

.flow-step--active .flow-step__index {
  color: var(--vp-c-brand-1);
}

.flow-step__heading {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.flow-step__title {
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--vp-c-text-1);
}

.flow-step__summary {
  font-size: 14px;
  line-height: 22px;
  color: var(--vp-c-text-2);
}

.flow-step__panel {
  border-top: 1px solid var(--vp-c-divider);
}

.flow-step__terminal {
  padding: 18px 22px 20px 58px;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  line-height: 1.75;
  background: var(--vp-code-block-bg, var(--vp-c-bg-soft));
  overflow-x: auto;
}

.flow-step__line {
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--vp-c-text-3);
}

.flow-step__line--command {
  color: var(--vp-c-text-1);
  font-weight: 500;
}

.flow-step__line--command:not(:first-child) {
  margin-top: 10px;
}

.flow-step__prompt {
  color: var(--vp-c-brand-1);
}

.flow-step__line--condition {
  margin-top: 2px;
  color: var(--vp-c-brand-1);
  font-weight: 600;
}

@media (prefers-reduced-motion: reduce) {
  .flow-step {
    transition: none;
  }
}
</style>
