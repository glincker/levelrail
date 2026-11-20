import { escapeHtml, page, type Starter } from './shared'

export const blankStarter: Starter = {
  id: 'blank',
  files: (name) => {
    const n = escapeHtml(name)
    return [
      {
        path: 'index.html',
        content: page(
          n,
          `<main class="wrap"><h1>${n}</h1><p class="muted">Replace this page with your own.</p></main>`,
          'main{padding:4rem 0}',
        ),
      },
    ]
  },
}
