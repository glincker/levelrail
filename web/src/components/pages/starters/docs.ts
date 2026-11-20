import { escapeHtml, page, type Starter } from './shared'

const CSS = `.layout{display:grid;gap:2rem;grid-template-columns:12rem 1fr;padding:2rem 0}
nav{position:sticky;top:1rem;align-self:start}
nav ul{list-style:none;margin:0;padding:0}
nav a{display:block;padding:.3rem 0;text-decoration:none}
pre{background:var(--card);border:1px solid var(--line);border-radius:.5rem;padding:1rem;overflow:auto}
h1{margin-top:0}
@media(max-width:40rem){.layout{grid-template-columns:1fr}nav{position:static}}`

export const docsStarter: Starter = {
  id: 'docs',
  files: (name) => {
    const n = escapeHtml(name)
    const body = `<div class="wrap layout">
<nav aria-label="Docs">
<strong>${n}</strong>
<ul>
<li><a href="#intro">Introduction</a></li>
<li><a href="#install">Install</a></li>
<li><a href="#usage">Usage</a></li>
</ul>
</nav>
<main>
<h1 id="intro">Introduction</h1>
<p class="muted">What this project is and who should read these docs.</p>
<h2 id="install">Install</h2>
<pre><code>npm install ${n}</code></pre>
<h2 id="usage">Usage</h2>
<p>Show the smallest working example first, then link to deeper guides.</p>
</main>
</div>`
    return [{ path: 'index.html', content: page(`${n} docs`, body, CSS) }]
  },
}
