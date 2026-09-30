<script setup lang="ts">
import { reactive } from 'vue'
import { useData } from 'vitepress'
import { useSidebar } from 'vitepress/theme'
import type { DefaultTheme } from 'vitepress/theme'
import { PhCaretRight } from '@phosphor-icons/vue'
import { sectionIcons, defaultSectionIcon } from './icons'

const { page } = useData()
const { sidebarGroups } = useSidebar()

function normalize(path: string): string {
  return path.replace(/\.md$/, '').replace(/(^|\/)index$/, '$1').replace(/^\/|\/$/g, '')
}

function isActive(link?: string): boolean {
  if (!link) return false
  return normalize(page.value.relativePath) === normalize(link)
}

function hasActiveDescendant(item: DefaultTheme.SidebarItem): boolean {
  if (isActive(item.link)) return true
  return (item.items ?? []).some(hasActiveDescendant)
}

// Collapsed state keyed by group text, seeded from each item's own
// `collapsed` field but forced open when it contains the active page.
const openState = reactive(new Map<string, boolean>())

function isOpen(item: DefaultTheme.SidebarItem, key: string): boolean {
  if (hasActiveDescendant(item)) return true
  if (!openState.has(key)) return item.collapsed !== true
  return openState.get(key)!
}

function toggle(key: string, item: DefaultTheme.SidebarItem) {
  openState.set(key, !isOpen(item, key))
}
</script>

<template>
  <nav class="custom-sidebar" aria-label="Docs navigation">
    <div v-for="(group, gi) in sidebarGroups" :key="group.text ?? gi" class="sidebar-section">
      <div class="sidebar-section__heading">
        <component
          :is="group.text ? (sectionIcons[group.text] ?? defaultSectionIcon) : defaultSectionIcon"
          class="sidebar-section__icon"
          :size="16"
          weight="bold"
        />
        <span>{{ group.text }}</span>
      </div>

      <ul class="sidebar-list">
        <li v-for="(item, ii) in group.items" :key="item.link ?? item.text ?? ii">
          <template v-if="item.items?.length">
            <button
              type="button"
              class="sidebar-group-toggle"
              :class="{ 'is-active': hasActiveDescendant(item) }"
              @click="toggle(`${gi}-${ii}`, item)"
            >
              <span>{{ item.text }}</span>
              <PhCaretRight
                class="sidebar-group-toggle__caret"
                :class="{ 'is-open': isOpen(item, `${gi}-${ii}`) }"
                :size="14"
                weight="bold"
              />
            </button>
            <ul v-show="isOpen(item, `${gi}-${ii}`)" class="sidebar-list sidebar-list--nested">
              <li v-for="(sub, si) in item.items" :key="sub.link ?? si">
                <a
                  :href="sub.link"
                  class="sidebar-link"
                  :class="{ 'is-active': isActive(sub.link) }"
                >
                  {{ sub.text }}
                </a>
              </li>
            </ul>
          </template>
          <a
            v-else
            :href="item.link"
            class="sidebar-link"
            :class="{ 'is-active': isActive(item.link) }"
          >
            {{ item.text }}
          </a>
        </li>
      </ul>
    </div>
  </nav>
</template>
