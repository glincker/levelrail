---
title: Announcements
description: "News about Levelrail: new guides, the self-host template gallery, and platform updates. Subscribe by RSS, Atom or JSON Feed."
---

# Announcements

Product news and site updates. Release notes live in the [changelog](/changelog/).

Subscribe: [RSS](/feed.xml), [Atom](/atom.xml) or [JSON Feed](/feed.json). The feeds also include every release.

<script setup>
import { data as posts } from './posts.data.mts'
</script>

<ul>
  <li v-for="post in posts" :key="post.url">
    <a :href="post.url">{{ post.title }}</a>
    <time :datetime="post.date"> {{ post.date }}</time>
    <p>{{ post.description }}</p>
  </li>
</ul>
