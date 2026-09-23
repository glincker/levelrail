import type { TokenizerAndRendererExtension, Tokens } from 'marked'

interface ContainerToken extends Tokens.Generic {
  type: 'container'
  containerType: string
  title: string
}

const OPEN_RE =
  /^:::[ \t]*(tip|warning|danger|info|details)\b[ \t]*([^\n]*)\n([\s\S]*?)\n:::[ \t]*(?:\n+|$)/

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

const CALLOUT_CLASSES: Record<string, string> = {
  tip: 'border-emerald-500 bg-emerald-500/10',
  warning: 'border-amber-500 bg-amber-500/10',
  danger: 'border-destructive bg-destructive/10',
  info: 'border-sky-500 bg-sky-500/10',
}

function renderCallout(type: string, title: string, inner: string): string {
  const label = title || type.charAt(0).toUpperCase() + type.slice(1)
  const cls = CALLOUT_CLASSES[type] ?? CALLOUT_CLASSES.info
  return `<div class="my-4 rounded-md border-l-4 p-3 text-sm ${cls}"><p class="mb-1 font-medium text-foreground">${escapeHtml(label)}</p><div class="text-muted-foreground">${inner}</div></div>`
}

function renderDetails(title: string, inner: string): string {
  return `<details class="my-4 rounded-md border border-border p-3 text-sm"><summary class="cursor-pointer font-medium text-foreground">${escapeHtml(title)}</summary><div class="mt-2 text-muted-foreground">${inner}</div></details>`
}

// VitePress "::: tip" / "::: warning" / "::: danger" / "::: info" /
// "::: details <summary>" containers, as a real marked block extension
// (recursively lexed children, not string surgery on rendered HTML) so
// nested markdown, code fences, and lists inside a container render
// correctly. This docs set never nests one container inside another, so
// a single non-greedy match up to the next bare ":::" line is enough.
export const vitepressContainerExtension: TokenizerAndRendererExtension = {
  name: 'container',
  level: 'block',
  tokenizer(src) {
    const match = OPEN_RE.exec(src)
    if (!match) return undefined
    const token: ContainerToken = {
      type: 'container',
      raw: match[0],
      containerType: match[1] as string,
      title: (match[2] as string).trim(),
      tokens: [],
    }
    this.lexer.blockTokens(match[3] as string, token.tokens)
    return token
  },
  renderer(genericToken) {
    const token = genericToken as ContainerToken
    const inner = this.parser.parse(token.tokens ?? [])
    return token.containerType === 'details'
      ? renderDetails(token.title, inner)
      : renderCallout(token.containerType, token.title, inner)
  },
}
