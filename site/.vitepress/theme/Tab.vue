<script setup lang="ts">
import { inject } from 'vue'
import type { Ref } from 'vue'

const props = defineProps<{
  value: string
}>()

// Set by the enclosing Tabs; absent only if Tab is used outside Tabs.
const active = inject<Ref<string>>('vp-tabs-active')
</script>

<template>
  <div v-if="active?.value === props.value" class="vp-tabs__panel" role="tabpanel">
    <slot />
  </div>
</template>
