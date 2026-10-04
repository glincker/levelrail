<script setup lang="ts">
import { ref } from 'vue'
import { useData } from 'vitepress'
import { PhCopy, PhCheck, PhCaretDown, PhMarkdownLogo } from '@phosphor-icons/vue'
import github from 'thesvg/github'
import chatgpt from 'thesvg/openai-chatgpt'
import claude from 'thesvg/claude-ai'
import perplexity from 'thesvg/perplexity'

const { page, theme } = useData()

const mdUrl = () => `${theme.value.siteUrl}/${page.value.relativePath}`
const githubUrl = () =>
  `https://github.com/glincker/levelrail/blob/main/docs/${page.value.relativePath}`

const copied = ref(false)

async function copyMarkdown() {
  try {
    const res = await fetch(`/${page.value.relativePath}`)
    await navigator.clipboard.writeText(await res.text())
    copied.value = true
    setTimeout(() => (copied.value = false), 1800)
  } catch {
    // Clipboard/network access denied (e.g. insecure context) -- fall
    // back to just opening the raw file so the content is still reachable.
    window.open(`/${page.value.relativePath}`, '_blank')
  }
}

function prompt() {
  return `Read ${mdUrl()} and help me understand it.`
}

// Brand marks (github/chatgpt/claude/perplexity) are raw SVG from thesvg,
// rendered via v-html; View as Markdown has no brand to represent, so it
// gets a plain Phosphor icon component instead.
const openLinks = () => [
  { label: 'Open in GitHub', href: githubUrl(), svg: github.svg },
  { label: 'View as Markdown', href: `/${page.value.relativePath}`, component: PhMarkdownLogo },
  { label: 'Open in ChatGPT', href: `https://chatgpt.com/?q=${encodeURIComponent(prompt())}`, svg: chatgpt.svg },
  { label: 'Open in Claude', href: `https://claude.ai/new?q=${encodeURIComponent(prompt())}`, svg: claude.svg },
  { label: 'Open in Perplexity', href: `https://www.perplexity.ai/search?q=${encodeURIComponent(prompt())}`, svg: perplexity.svg },
]
</script>

<template>
  <div class="page-actions">
    <button type="button" class="page-actions__btn" @click="copyMarkdown">
      <PhCheck v-if="copied" :size="14" weight="bold" />
      <PhCopy v-else :size="14" weight="bold" />
      {{ copied ? 'Copied!' : 'Copy Markdown' }}
    </button>

    <details class="page-actions__dropdown">
      <summary class="page-actions__btn">
        Open
        <PhCaretDown :size="12" weight="bold" />
      </summary>
      <div class="page-actions__menu">
        <a
          v-for="item in openLinks()"
          :key="item.label"
          class="page-actions__menu-item"
          :href="item.href"
          target="_blank"
          rel="noreferrer"
        >
          <span v-if="item.svg" class="page-actions__menu-icon" v-html="item.svg" />
          <component :is="item.component" v-else class="page-actions__menu-icon" :size="16" />
          {{ item.label }}
        </a>
      </div>
    </details>
  </div>
</template>
