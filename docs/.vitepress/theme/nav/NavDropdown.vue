<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { PhCaretDown } from '@phosphor-icons/vue'
import NavMenuItem from './NavMenuItem.vue'
import type { NavGroup } from './navItems'

const OPEN_DELAY_MS = 90
const CLOSE_DELAY_MS = 220
const VIEWPORT_MARGIN = 16

const props = defineProps<{ group: NavGroup; id: string; open: boolean; anyOpen: boolean }>()
const emit = defineEmits<{ open: []; close: [] }>()

const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const panel = ref<HTMLElement | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined

function clearTimer() {
  if (timer) clearTimeout(timer)
  timer = undefined
}

function items(): HTMLElement[] {
  return Array.from(panel.value?.querySelectorAll<HTMLElement>('[data-nav-item]') ?? [])
}

function focusItem(index: number) {
  const list = items()
  if (!list.length) return
  list[(index + list.length) % list.length].focus()
}

function onPointerEnter(e: PointerEvent) {
  if (e.pointerType !== 'mouse') return
  clearTimer()
  if (props.open) return
  timer = setTimeout(() => emit('open'), props.anyOpen ? 0 : OPEN_DELAY_MS)
}

function onPointerLeave(e: PointerEvent) {
  if (e.pointerType !== 'mouse') return
  clearTimer()
  if (!props.open) return
  timer = setTimeout(() => emit('close'), CLOSE_DELAY_MS)
}

function onTriggerClick() {
  clearTimer()
  emit(props.open ? 'close' : 'open')
}

async function openAndFocus(index: number) {
  emit('open')
  await nextTick()
  focusItem(index)
}

function onTriggerKeydown(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    openAndFocus(0)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    openAndFocus(-1)
  }
}

function onPanelKeydown(e: KeyboardEvent) {
  const list = items()
  const at = list.indexOf(document.activeElement as HTMLElement)
  switch (e.key) {
    case 'ArrowDown':
    case 'ArrowRight':
      e.preventDefault()
      focusItem(at + 1)
      break
    case 'ArrowUp':
    case 'ArrowLeft':
      e.preventDefault()
      focusItem(at - 1)
      break
    case 'Home':
      e.preventDefault()
      focusItem(0)
      break
    case 'End':
      e.preventDefault()
      focusItem(-1)
      break
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.open) {
    e.stopPropagation()
    emit('close')
    trigger.value?.focus()
  }
}

function onFocusOut(e: FocusEvent) {
  const next = e.relatedTarget as Node | null
  if (props.open && next && !root.value?.contains(next)) emit('close')
}

// Clamp the panel inside the viewport; the shift is a CSS variable so styling stays in the stylesheet.
async function clampToViewport() {
  await nextTick()
  const el = panel.value
  if (!el) return
  el.style.setProperty('--nav-panel-shift', '0px')
  const rect = el.getBoundingClientRect()
  const max = window.innerWidth - VIEWPORT_MARGIN
  let shift = 0
  if (rect.right > max) shift = max - rect.right
  if (rect.left + shift < VIEWPORT_MARGIN) shift = VIEWPORT_MARGIN - rect.left
  el.style.setProperty('--nav-panel-shift', `${Math.round(shift)}px`)
}

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) clampToViewport()
  },
)

onBeforeUnmount(clearTimer)
</script>

<template>
  <div
    ref="root"
    class="nav-dropdown"
    @pointerenter="onPointerEnter"
    @pointerleave="onPointerLeave"
    @keydown="onKeydown"
    @focusout="onFocusOut"
  >
    <button
      ref="trigger"
      type="button"
      class="custom-nav__link nav-dropdown__trigger"
      :class="{ 'is-open': open }"
      aria-haspopup="true"
      :aria-expanded="open"
      :aria-controls="`${id}-panel`"
      @click="onTriggerClick"
      @keydown="onTriggerKeydown"
    >
      <span class="nav-dropdown__dot" aria-hidden="true" />
      {{ group.label }}
      <PhCaretDown class="nav-dropdown__caret" :size="11" weight="bold" aria-hidden="true" />
    </button>
    <div
      :id="`${id}-panel`"
      ref="panel"
      class="nav-dropdown__panel"
      :class="{ 'is-open': open }"
      role="group"
      :aria-label="group.label"
      :inert="!open"
      @keydown="onPanelKeydown"
    >
      <div class="nav-dropdown__grid">
        <NavMenuItem v-for="item in group.items" :key="item.link" :item="item" @navigate="emit('close')" />
      </div>
    </div>
  </div>
</template>
