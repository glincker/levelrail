---
title: Self-host gallery
description: Browse self-hostable open-source apps you can deploy with Levelrail templates, with the services, ports, volumes and environment variables for each.
outline: false
---

<script setup>
import { computed, ref } from 'vue'
import { data as cards } from './templates.data.mts'

const query = ref('')
const category = ref('')
const categories = [...new Set(cards.map((c) => c.category))].sort()
const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  return cards.filter(
    (c) =>
      (!category.value || c.category === category.value) &&
      (!q || c.name.toLowerCase().includes(q) || c.slogan.toLowerCase().includes(q)),
  )
})
</script>

# Self-host gallery

{{ cards.length }} open-source apps you can run on your own server with a Levelrail template. Each page lists the services, ports, volumes and environment variables the template sets up, and how to deploy it. For how templates work, see the [service template catalog](/templates-and-registry).

<div class="gallery-controls">
  <input v-model="query" type="search" placeholder="Search templates" aria-label="Search templates" />
  <select v-model="category" aria-label="Filter by category">
    <option value="">All categories</option>
    <option v-for="c in categories" :key="c" :value="c">{{ c }}</option>
  </select>
</div>

<p>{{ shown.length }} shown</p>

<ul class="gallery-list">
  <li v-for="c in shown" :key="c.id">
    <a :href="`/self-host/${c.id}`"><strong>{{ c.name }}</strong></a>
    <span class="gallery-cat">{{ c.category }}</span>
    <div>{{ c.slogan }}</div>
  </li>
</ul>

<style scoped>
.gallery-controls { display: flex; gap: 12px; flex-wrap: wrap; margin: 16px 0; }
.gallery-controls input, .gallery-controls select {
  padding: 8px 12px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
  font: inherit;
}
.gallery-controls input { flex: 1 1 240px; }
.gallery-list { list-style: none; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; }
.gallery-list li { border: 1px solid var(--vp-c-divider); border-radius: 8px; padding: 12px 14px; margin: 0; }
.gallery-cat { margin-left: 8px; font-size: 12px; color: var(--vp-c-text-2); }
.gallery-list div { font-size: 14px; color: var(--vp-c-text-2); margin-top: 4px; }
</style>
