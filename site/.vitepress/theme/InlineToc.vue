<script setup lang="ts">
import { ref, onMounted, nextTick, watch } from 'vue'
import { useRoute } from 'vitepress'
import { PhCaretDown } from '@phosphor-icons/vue'

const props = withDefaults(
  defineProps<{
    title?: string
    defaultOpen?: boolean
  }>(),
  { title: 'On this page', defaultOpen: false },
)

interface Heading {
  id: string
  text: string
  level: number
}

const headings = ref<Heading[]>([])
const isOpen = ref(props.defaultOpen)
const route = useRoute()

// No public VitePress API hands a custom component the current page's
// heading list, so this reads the rendered h2/h3s directly (same approach
// VitePress's own sidebar outline takes internally).
function collect() {
  const container = document.querySelector('.vp-doc')
  if (!container) return
  const nodes = container.querySelectorAll('h2, h3')
  headings.value = Array.from(nodes)
    .filter((el): el is HTMLHeadingElement => el.id.length > 0)
    .map((el) => ({
      id: el.id,
      text: el.textContent?.replace(/\s*#\s*$/, '').trim() ?? '',
      level: el.tagName === 'H2' ? 2 : 3,
    }))
}

onMounted(() => nextTick(collect))
watch(
  () => route.path,
  () => nextTick(collect),
)
</script>

<template>
  <div v-if="headings.length > 0" class="vp-inline-toc" :class="{ 'vp-inline-toc--open': isOpen }">
    <button type="button" class="vp-inline-toc__header" :aria-expanded="isOpen" @click="isOpen = !isOpen">
      <span class="vp-inline-toc__title">{{ title }}</span>
      <PhCaretDown class="vp-inline-toc__caret" weight="bold" size="16" />
    </button>
    <ul v-show="isOpen" class="vp-inline-toc__list">
      <li
        v-for="h in headings"
        :key="h.id"
        class="vp-inline-toc__item"
        :class="`vp-inline-toc__item--level-${h.level}`"
      >
        <a :href="`#${h.id}`">{{ h.text }}</a>
      </li>
    </ul>
  </div>
</template>
