<script setup lang="ts">
import { ref, provide } from 'vue'

const props = defineProps<{
  items: string[]
  defaultValue?: string
}>()

const active = ref(props.defaultValue ?? props.items[0])
provide('vp-tabs-active', active)

function select(item: string) {
  active.value = item
}

function onKeydown(event: KeyboardEvent, index: number) {
  if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return
  event.preventDefault()
  const delta = event.key === 'ArrowRight' ? 1 : -1
  const next = (index + delta + props.items.length) % props.items.length
  active.value = props.items[next]
}
</script>

<template>
  <div class="vp-tabs">
    <div class="vp-tabs__list" role="tablist">
      <button
        v-for="(item, i) in items"
        :key="item"
        type="button"
        role="tab"
        class="vp-tabs__tab"
        :class="{ 'vp-tabs__tab--active': active === item }"
        :aria-selected="active === item"
        :tabindex="active === item ? 0 : -1"
        @click="select(item)"
        @keydown="onKeydown($event, i)"
      >
        {{ item }}
      </button>
    </div>
    <slot />
  </div>
</template>
