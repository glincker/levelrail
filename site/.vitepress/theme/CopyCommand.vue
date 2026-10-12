<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { PhCheck, PhCopy } from '@phosphor-icons/vue'

const props = defineProps<{ command: string; prompt?: string }>()

const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

async function copy() {
  try {
    await navigator.clipboard.writeText(props.command)
    copied.value = true
    clearTimeout(timer)
    timer = setTimeout(() => (copied.value = false), 2000)
  } catch {
    copied.value = false
  }
}

onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div class="copy-cmd">
    <span v-if="prompt" class="copy-cmd__prompt" aria-hidden="true">{{ prompt }}</span>
    <code class="copy-cmd__text">{{ command }}</code>
    <button type="button" class="copy-cmd__btn" :aria-label="copied ? 'Copied' : 'Copy install command'" @click="copy">
      <PhCheck v-if="copied" weight="bold" class="copy-cmd__icon" />
      <PhCopy v-else weight="bold" class="copy-cmd__icon" />
    </button>
    <span class="copy-cmd__status" role="status" aria-live="polite">{{ copied ? 'Copied' : '' }}</span>
  </div>
</template>
