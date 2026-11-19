import { escapeHtml, page, type Starter } from './shared'

const CSS = `header,section,footer{padding:3rem 0}
.hero{padding:5rem 0 3rem}
h1{font-size:clamp(2rem,6vw,3.25rem);line-height:1.1;margin:0 0 1rem}
.grid{display:grid;gap:1rem;grid-template-columns:repeat(auto-fit,minmax(14rem,1fr))}
.card{background:var(--card);border:1px solid var(--line);border-radius:.75rem;padding:1.25rem}
.card h3{margin:0 0 .35rem}
footer{border-top:1px solid var(--line)}`

export const landingStarter: Starter = {
  id: 'landing',
  files: (name) => {
    const n = escapeHtml(name)
    const body = `<header class="wrap"><strong>${n}</strong></header>
<main>
<section class="wrap hero">
<h1>One clear line about what you make</h1>
<p class="muted">A short supporting sentence that says who it is for and why it helps.</p>
<p><a class="btn" href="#features">See what it does</a></p>
</section>
<section class="wrap" id="features" aria-labelledby="f">
<h2 id="f">Features</h2>
<div class="grid">
<div class="card"><h3>Fast</h3><p class="muted">Served as plain static files.</p></div>
<div class="card"><h3>Simple</h3><p class="muted">Edit the HTML and publish again.</p></div>
<div class="card"><h3>Yours</h3><p class="muted">Connect your own domain later.</p></div>
</div>
</section>
</main>
<footer class="wrap muted">&copy; ${n}</footer>`
    return [{ path: 'index.html', content: page(n, body, CSS) }]
  },
}
