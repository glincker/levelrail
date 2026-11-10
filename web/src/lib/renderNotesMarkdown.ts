import { Marked } from 'marked'
import type { Tokens } from 'marked'

// App and database notes are written by any editor and shown to every reader,
// so raw HTML and images are dropped and only http(s) and mailto links render.
const md = new Marked({ gfm: true, breaks: true })
md.use({
  renderer: {
    html() {
      return ''
    },
    image() {
      return ''
    },
    link(token: Tokens.Link) {
      const inner = this.parser.parseInline(token.tokens)
      if (!/^(https?:|mailto:)/i.test(token.href.trim())) return inner
      return `<a href="${escapeAttr(token.href.trim())}" target="_blank" rel="noopener noreferrer" class="text-primary underline underline-offset-2 hover:no-underline">${inner}</a>`
    },
  },
})

function escapeAttr(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

export function renderNotesMarkdown(markdown: string): string {
  return md.parse(markdown, { async: false })
}
