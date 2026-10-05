<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { PhCaretDown } from '@phosphor-icons/vue'
import NavMenuItem from './NavMenuItem.vue'
import type { NavEntry } from './navItems'

const props = defineProps<{ entries: NavEntry[]; open: boolean; path: string }>()
const emit = defineEmits<{ close: []; wrap: [] }>()

const sheet = ref<HTMLElement | null>(null)
const expanded = ref<Set<string>>(new Set())

const groups = computed(() =>
  props.entries.flatMap((e) => (e.kind === 'group' ? [e.group] : [])),
)
const plainLinks = computed(() =>
  props.entries.flatMap((e) => (e.kind === 'link' ? [e.link] : [])),
)

function toggle(label: string) {
  const next = new Set(expanded.value)
  if (next.has(label)) next.delete(label)
  else next.add(label)
  expanded.value = next
}

// Start with the group that owns the current page expanded.
watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return
    const current = groups.value.find((g) => g.items.some((i) => !i.external && props.path.startsWith(i.link.replace(/\/$/, ''))))
    expanded.value = new Set(current ? [current.label] : [])
  },
)

function focusables(): HTMLElement[] {
  return Array.from(
    sheet.value?.querySelectorAll<HTMLElement>('a[href], button:not([disabled])') ?? [],
  ).filter((el) => !el.closest('[inert]'))
}

function focusFirst() {
  focusables()[0]?.focus()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Tab') return
  const list = focusables()
  if (!list.length) return
  const first = list[0]
  const last = list[list.length - 1]
  if ((e.shiftKey && document.activeElement === first) || (!e.shiftKey && document.activeElement === last)) {
    e.preventDefault()
    emit('wrap')
  }
}

defineExpose({ focusFirst })
</script>

<template>
  <Teleport to="body">
    <div
      id="nav-mobile-sheet"
      ref="sheet"
      class="nav-sheet"
      :class="{ 'is-open': open }"
      role="dialog"
      aria-label="Site navigation"
      :inert="!open"
      @keydown="onKeydown"
    >
      <nav class="nav-sheet__inner" aria-label="Site">
        <section v-for="group in groups" :key="group.label" class="nav-sheet__group">
          <h2 class="nav-sheet__heading">
            <button
              type="button"
              class="nav-sheet__toggle"
              :aria-expanded="expanded.has(group.label)"
              :aria-controls="`nav-sheet-${group.label}`"
              @click="toggle(group.label)"
            >
              <span class="nav-dropdown__dot" aria-hidden="true" />
              {{ group.label }}
              <PhCaretDown class="nav-sheet__caret" :size="14" weight="bold" aria-hidden="true" />
            </button>
          </h2>
          <div
            :id="`nav-sheet-${group.label}`"
            class="nav-sheet__region"
            :class="{ 'is-open': expanded.has(group.label) }"
            :inert="!expanded.has(group.label)"
          >
            <div class="nav-sheet__list">
              <NavMenuItem v-for="item in group.items" :key="item.link" :item="item" @navigate="emit('close')" />
            </div>
          </div>
        </section>
        <a
          v-for="link in plainLinks"
          :key="link.link"
          class="nav-sheet__plain"
          :href="link.link"
          @click="emit('close')"
        >
          <span class="nav-dropdown__dot" aria-hidden="true" />
          {{ link.text }}
        </a>
      </nav>
    </div>
  </Teleport>
</template>
