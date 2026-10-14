<script setup lang="ts">
import { ref } from 'vue'
import { PhCaretDown } from '@phosphor-icons/vue'
import { faqItems as items } from './faqData'

const openIndex = ref<number | null>(0)

function toggle(i: number) {
  openIndex.value = openIndex.value === i ? null : i
}
</script>

<template>
  <div class="faq-list">
    <div
      v-for="(item, i) in items"
      :key="item.q"
      class="faq-item"
      :class="{ 'faq-item--open': openIndex === i }"
    >
      <button
        type="button"
        class="faq-item__header"
        :aria-expanded="openIndex === i"
        :aria-controls="`faq-panel-${i}`"
        @click="toggle(i)"
      >
        <span class="faq-item__question">{{ item.q }}</span>
        <PhCaretDown class="faq-item__caret" weight="bold" />
      </button>
      <div
        v-show="openIndex === i"
        :id="`faq-panel-${i}`"
        class="faq-item__panel"
        role="region"
      >
        <p class="faq-item__answer">{{ item.a }}</p>
      </div>
    </div>
  </div>
</template>
