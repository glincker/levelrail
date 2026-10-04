import { Marked } from 'marked'
import type { Tokens } from 'marked'

// A dedicated, locked-down Marked instance for AI assistant replies,
// separate from lib/renderDocsMarkdown.ts's instance: docs content is
// authored by this team and trusted, but an assistant reply can echo
// attacker-controlled text pulled in through a tool call (logs, env
// values, a webhook payload), per internal/ai/engine.go's own untrusted-
// data-block system prompt warning. Raw HTML and images are dropped
// outright rather than escaped-and-shown, and only http(s)/mailto/
// relative links render as actual links, so markdown syntax can never
// become a script tag, an inline event handler, or a javascript: link.
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
      if (
        !/^(https?:|mailto:)/i.test(token.href) &&
        !token.href.startsWith('/')
      ) {
        return inner
      }
      return `<a href="${token.href}" target="_blank" rel="noreferrer noopener" class="text-primary underline underline-offset-2 hover:no-underline">${inner}</a>`
    },
  },
})

export function renderChatMarkdown(markdown: string): string {
  return md.parse(markdown, { async: false })
}
