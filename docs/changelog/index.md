---
title: Changelog
description: Every Levelrail release with highlights, fixes, upgrade notes, container image digests, and verification steps. Subscribe via the Atom feed.
---

<script setup>
import { data as releases } from './releases.data.mts'
</script>

# Changelog

Every release, newest first. Each page lists what changed, upgrade notes, the exact container image digests, and how to verify the download. Subscribe with the [Atom feed](/changelog/feed.xml) or watch releases on [GitHub](https://github.com/glincker/levelrail/releases).

<div v-for="r in releases" :key="r.tag" class="changelog-entry">
  <h2 :id="r.slug"><a :href="`/changelog/${r.slug}`">{{ r.tag }}</a></h2>
  <p class="changelog-meta"><time :datetime="r.date">{{ r.date }}</time><span v-if="r.prerelease">, pre-release</span></p>
  <ul v-if="r.highlights.length">
    <li v-for="h in r.highlights" :key="h">{{ h }}</li>
  </ul>
</div>
