<script setup lang="ts">
import DefaultTheme from 'vitepress/theme'

const { Layout } = DefaultTheme
</script>

<template>
  <Layout>
    <template #layout-bottom>
      <div class="site-footer-extra">
        <div class="site-footer-extra__inner">
          <div class="site-footer-extra__col">
            <p class="site-footer-extra__heading">Product</p>
            <a href="/getting-started">Getting started</a>
            <a href="/comparison">Compare</a>
            <a href="/roadmap">Roadmap</a>
            <a href="/troubleshooting">Troubleshooting</a>
          </div>
          <div class="site-footer-extra__col">
            <p class="site-footer-extra__heading">Reference</p>
            <a href="/app-spec-reference">App spec reference</a>
            <a href="/cli-reference">CLI reference</a>
            <a href="/api-reference">API reference</a>
            <a href="/security">Security overview</a>
          </div>
          <div class="site-footer-extra__col">
            <p class="site-footer-extra__heading">Community</p>
            <a href="https://github.com/glincker/levelrail" target="_blank" rel="noreferrer">GitHub</a>
            <a href="https://github.com/glincker/levelrail/issues" target="_blank" rel="noreferrer">Issues</a>
            <a href="https://github.com/glincker/levelrail/discussions" target="_blank" rel="noreferrer">Discussions</a>
          </div>
          <div class="site-footer-extra__col">
            <p class="site-footer-extra__heading">Levelrail</p>
            <p class="site-footer-extra__blurb">
              A self-hosted deployment platform. Apache 2.0.
            </p>
            <a href="https://glincker.com" target="_blank" rel="noreferrer" class="site-footer-extra__badge">
              A GLINCKER project
            </a>
          </div>
        </div>
      </div>
    </template>
  </Layout>
</template>
