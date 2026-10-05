---
title: Product mock preview
sidebar: false
aside: false
head:
  - - meta
    - name: robots
      content: noindex
---

# Product mock preview

Review page for the product mock kit. Not linked from navigation.

<div class="mp-grid">
  <template v-for="v in ['apps', 'overview', 'deploys', 'logs']" :key="v">
    <h2>{{ v }} (dark)</h2>
    <ProductMock :view="v" theme="dark" animate />
    <h2>{{ v }} (light)</h2>
    <ProductMock :view="v" theme="light" animate />
    <h2>{{ v }} (dark, cropped)</h2>
    <ProductMock :view="v" theme="dark" cropped />
    <h2>{{ v }} (light, cropped)</h2>
    <ProductMock :view="v" theme="light" cropped />
  </template>
</div>

<script setup lang="ts">
import ProductMock from './.vitepress/theme/mock/ProductMock.vue'
</script>
