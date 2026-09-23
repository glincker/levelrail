---
outline: [2, 3]
---

# Levelrail {{ $params.tag }} {#release}

<p class="changelog-meta">
  Released <time :datetime="$params.date">{{ $params.date }}</time>
  <span v-if="$params.prerelease"> as a pre-release</span>.
  <a :href="$params.url">View on GitHub</a>. <a href="/changelog/">All releases</a>.
</p>

<!-- @content -->
