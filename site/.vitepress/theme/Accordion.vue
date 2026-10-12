<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { PhCaretDown, PhLink, PhCheck } from '@phosphor-icons/vue'

const props = withDefaults(
  defineProps<{
    title: string
    id?: string
    open?: boolean
  }>(),
  { open: false },
)

const isOpen = ref(props.open)
const copied = ref(false)

function toggle() {
  isOpen.value = !isOpen.value
}

async function copyLink(event: MouseEvent) {
  event.stopPropagation()
  if (!props.id) return
  const url = new URL(window.location.href)
  url.hash = props.id
  await navigator.clipboard.writeText(url.toString())
  copied.value = true
  setTimeout(() => (copied.value = false), 1500)
}

// Deep link: /page#the-accordion-id opens that entry pre-expanded, the
// same behavior fumadocs' Accordion ports from its own hash-scan on mount.
onMounted(() => {
  if (props.id && window.location.hash === `#${props.id}`) {
    isOpen.value = true
  }
})
</script>

<template>
  <div :id="id" class="vp-accordion" :class="{ 'vp-accordion--open': isOpen }">
    <button type="button" class="vp-accordion__header" :aria-expanded="isOpen" @click="toggle">
      <span class="vp-accordion__title">{{ title }}</span>
      <span class="vp-accordion__actions">
        <button
          v-if="id"
          type="button"
          class="vp-accordion__copy"
          aria-label="Copy link to this section"
          @click="copyLink"
        >
          <PhCheck v-if="copied" weight="bold" size="13" />
          <PhLink v-else weight="bold" size="13" />
        </button>
        <PhCaretDown class="vp-accordion__caret" weight="bold" size="16" />
      </span>
    </button>
    <div v-show="isOpen" class="vp-accordion__panel">
      <slot />
    </div>
  </div>
</template>
