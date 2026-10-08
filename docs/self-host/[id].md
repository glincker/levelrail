---
outline: [2, 2]
---

<script setup>
import { useData } from 'vitepress'
const { params } = useData()
const kindLabel = { generated: 'Generated at deploy', required: 'You must provide', preset: 'Preset in the template' }
</script>

# Self-host {{ params.name }} with Docker

{{ params.slogan }}

This page describes the Levelrail template for {{ params.name }} ({{ params.category }}). It deploys the services below as one Docker Compose app on your own server.
<template v-if="params.docUrl"> Project site: <a :href="params.docUrl" rel="noopener">{{ params.docUrl }}</a>.</template>

<p v-if="params.memoryMiB">Recommended memory: about {{ params.memoryMiB }} MiB.</p>
<p v-if="params.gpu">This template reserves an NVIDIA GPU, so it needs a node with a GPU and the NVIDIA container runtime.</p>

## Services, ports and volumes

<table>
  <thead><tr><th>Service</th><th>Image</th><th>Container ports</th><th>Volumes</th></tr></thead>
  <tbody>
    <tr v-for="s in params.services" :key="s.name">
      <td><code>{{ s.name }}</code></td>
      <td><code>{{ s.image }}</code></td>
      <td>{{ s.ports.length ? s.ports.join(', ') : 'none' }}</td>
      <td><template v-if="s.volumes.length"><div v-for="v in s.volumes" :key="v"><code>{{ v }}</code></div></template><template v-else>none</template></td>
    </tr>
  </tbody>
</table>

## Environment variables

<p v-if="!params.env.length">This template sets no environment variables.</p>
<table v-else>
  <thead><tr><th>Service</th><th>Variable</th><th>Value</th></tr></thead>
  <tbody>
    <tr v-for="e in params.env" :key="e.service + e.key">
      <td><code>{{ e.service }}</code></td>
      <td><code>{{ e.key }}</code></td>
      <td>{{ kindLabel[e.kind] }}</td>
    </tr>
  </tbody>
</table>

Passwords and keys marked as generated are created for you when the app is deployed and stored as secrets. Values are not shown here.

<p v-if="params.needsConfig"><strong>This template needs configuration before it can deploy.</strong> Open it in the dashboard, set each variable marked "You must provide" in the Compose body, then deploy.</p>

## Deploy {{ params.name }} with Levelrail

In the dashboard, open **Apps**, choose **New app**, then **Browse templates**, and select {{ params.name }}. Review the Compose body and deploy.

With the CLI:

<pre><code v-if="!params.needsConfig">levelrail-cli templates deploy {{ params.id }} --name my-{{ params.id }}</code><code v-else>levelrail-cli templates get {{ params.id }} --output json | jq -r .compose > {{ params.id }}.compose.yaml
# edit {{ params.id }}.compose.yaml to set the variables you must provide
levelrail-cli apps deploy-compose my-{{ params.id }} --file {{ params.id }}.compose.yaml</code></pre>

See [Service template catalog](/templates-and-registry) for how templates work, and [Getting started](/getting-started) if you have not installed Levelrail yet.

<template v-if="params.related.length">

## More {{ params.category }} templates

<ul>
  <li v-for="r in params.related" :key="r.id"><a :href="`/self-host/${r.id}`">{{ r.name }}</a></li>
</ul>

</template>

All templates are listed in the [self-host gallery](/self-host/).
