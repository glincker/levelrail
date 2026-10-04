<script setup lang="ts">
import { computed } from 'vue'
import { data as releases } from '../../changelog/releases.data.mts'

// releases.data.mts already sorts newest first (loadReleases in
// .vitepress/releases.mts sorts by published date descending); slice only.
const latest = computed(() => releases.slice(0, 4))

function excerpt(highlights: string[]): string {
  if (!highlights.length) return 'Release notes, upgrade steps, and verification details.'
  return highlights.slice(0, 2).join(', ')
}
</script>

<template>
  <div class="releases-feed">
    <ul class="releases-feed__list">
      <li v-for="r in latest" :key="r.tag" class="releases-feed__item">
        <a :href="`/changelog/${r.slug}`" class="releases-feed__link">
          <p class="releases-feed__meta">
            <time class="releases-feed__date" :datetime="r.date">{{ r.date }}</time>
            <span v-if="r.prerelease" class="releases-feed__badge">pre-release</span>
          </p>
          <h3 class="releases-feed__tag">{{ r.tag }}</h3>
          <p class="releases-feed__excerpt">{{ excerpt(r.highlights) }}</p>
        </a>
      </li>
    </ul>
    <a class="releases-feed__all" href="/changelog/">View all releases</a>
  </div>
</template>
