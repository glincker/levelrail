import { escapeHtml, page, type Starter } from './shared'

const CSS = `header{padding:4rem 0 2rem}
h1{font-size:clamp(2rem,6vw,3rem);margin:0}
.grid{display:grid;gap:1rem;grid-template-columns:repeat(auto-fill,minmax(15rem,1fr));padding:1rem 0 3rem}
.work{background:var(--card);border:1px solid var(--line);border-radius:.75rem;padding:1.25rem}
.work h3{margin:0 0 .35rem}
footer{border-top:1px solid var(--line);padding:2rem 0}`

export const portfolioStarter: Starter = {
  id: 'portfolio',
  files: (name) => {
    const n = escapeHtml(name)
    const body = `<header class="wrap">
<h1>${n}</h1>
<p class="muted">Designer and developer. A sentence about what you do and where.</p>
</header>
<main class="wrap">
<h2>Selected work</h2>
<div class="grid">
<article class="work"><h3>Project one</h3><p class="muted">What it was and what you did.</p></article>
<article class="work"><h3>Project two</h3><p class="muted">What it was and what you did.</p></article>
<article class="work"><h3>Project three</h3><p class="muted">What it was and what you did.</p></article>
</div>
</main>
<footer class="wrap"><a href="mailto:you@example.com">you@example.com</a></footer>`
    return [{ path: 'index.html', content: page(n, body, CSS) }]
  },
}
