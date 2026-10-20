import type { ReactNode } from 'react'
import { lexer, type Token, type Tokens } from 'marked'
import {
  InfoIcon,
  LightbulbIcon,
  WarningCircleIcon,
  WarningIcon,
  ChatCenteredTextIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import { stripNoteComments } from '../../lib/releaseNotes'

type AlertKind = 'note' | 'tip' | 'important' | 'warning' | 'caution'

const ALERT_RE = /^\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*\n?/i
const SAFE_HREF = /^(https?:\/\/|mailto:)/i

const ALERT_ICON: Record<AlertKind, ReactNode> = {
  note: <InfoIcon className="size-4" />,
  tip: <LightbulbIcon className="size-4" />,
  important: <ChatCenteredTextIcon className="size-4" />,
  warning: <WarningIcon className="size-4" />,
  caution: <WarningCircleIcon className="size-4" />,
}

const ALERT_TONE: Record<AlertKind, string> = {
  note: 'border-border',
  tip: 'border-border',
  important: 'border-primary/50',
  warning: 'border-amber-500/60',
  caution: 'border-destructive/60',
}

function inline(tokens: Token[] | undefined, keyBase: string): ReactNode[] {
  if (!tokens) return []
  return tokens.map((tok, i) => {
    const key = `${keyBase}-${i}`
    switch (tok.type) {
      case 'strong':
        return (
          <strong key={key}>
            {inline((tok as Tokens.Strong).tokens, key)}
          </strong>
        )
      case 'em':
        return <em key={key}>{inline((tok as Tokens.Em).tokens, key)}</em>
      case 'del':
        return <del key={key}>{inline((tok as Tokens.Del).tokens, key)}</del>
      case 'codespan':
        return (
          <code key={key} className="rounded bg-muted px-1 font-mono">
            {(tok as Tokens.Codespan).text}
          </code>
        )
      case 'br':
        return <br key={key} />
      case 'link': {
        const link = tok as Tokens.Link
        const children = inline(link.tokens, key)
        if (!SAFE_HREF.test(link.href)) return <span key={key}>{children}</span>
        return (
          <a
            key={key}
            href={link.href}
            target="_blank"
            rel="noopener noreferrer"
            className="text-primary underline underline-offset-2 hover:no-underline"
          >
            {children}
          </a>
        )
      }
      case 'html':
      case 'image':
        return null
      case 'text':
      case 'escape': {
        const text = tok as Tokens.Text
        return text.tokens ? (
          <span key={key}>{inline(text.tokens, key)}</span>
        ) : (
          <span key={key}>{text.text}</span>
        )
      }
      default:
        return null
    }
  })
}

function alertKind(quote: Tokens.Blockquote): AlertKind | null {
  const first = quote.tokens[0]
  if (first?.type !== 'paragraph') return null
  const m = ALERT_RE.exec((first as Tokens.Paragraph).text)
  if (!m?.[1]) return null
  return m[1].toLowerCase() as AlertKind
}

function AlertBlock({
  kind,
  quote,
  keyBase,
}: {
  kind: AlertKind
  quote: Tokens.Blockquote
  keyBase: string
}) {
  const { t } = useTranslation('updates')
  const [first, ...rest] = quote.tokens
  const bodyTokens = [
    ...((first as Tokens.Paragraph | undefined)?.tokens ?? []),
  ]
  const head = bodyTokens[0]
  if (head?.type === 'text') {
    const text = head as Tokens.Text
    bodyTokens[0] = {
      ...text,
      text: text.text.replace(ALERT_RE, ''),
      raw: text.raw.replace(ALERT_RE, ''),
    }
  }
  return (
    <aside
      className={`my-2 rounded-md border-l-2 bg-muted/50 px-3 py-2 ${ALERT_TONE[kind]}`}
    >
      <p className="mb-1 flex items-center gap-1.5 text-xs font-medium text-foreground">
        {ALERT_ICON[kind]}
        {t(`notes.alert.${kind}`)}
      </p>
      <div className="space-y-1">
        <p>{inline(bodyTokens, `${keyBase}-a`)}</p>
        {blocks(rest, `${keyBase}-r`)}
      </div>
    </aside>
  )
}

function blocks(tokens: Token[], keyBase: string): ReactNode[] {
  return tokens.map((tok, i) => {
    const key = `${keyBase}-${i}`
    switch (tok.type) {
      case 'heading': {
        const h = tok as Tokens.Heading
        return (
          <p key={key} className="pt-1 font-medium text-foreground">
            {inline(h.tokens, key)}
          </p>
        )
      }
      case 'paragraph':
        return <p key={key}>{inline((tok as Tokens.Paragraph).tokens, key)}</p>
      case 'text':
        return <p key={key}>{inline((tok as Tokens.Text).tokens, key)}</p>
      case 'list': {
        const list = tok as Tokens.List
        const Tag = list.ordered ? 'ol' : 'ul'
        return (
          <Tag
            key={key}
            className={`ml-4 space-y-0.5 ${list.ordered ? 'list-decimal' : 'list-disc'}`}
          >
            {list.items.map((item, j) => (
              <li key={`${key}-${j}`}>{blocks(item.tokens, `${key}-${j}`)}</li>
            ))}
          </Tag>
        )
      }
      case 'blockquote': {
        const q = tok as Tokens.Blockquote
        const kind = alertKind(q)
        if (kind)
          return <AlertBlock key={key} kind={kind} quote={q} keyBase={key} />
        return (
          <blockquote
            key={key}
            className="border-l-2 border-border pl-3 text-muted-foreground"
          >
            {blocks(q.tokens, key)}
          </blockquote>
        )
      }
      case 'code':
        return (
          <pre
            key={key}
            className="overflow-auto rounded bg-muted px-2 py-1 font-mono"
          >
            {(tok as Tokens.Code).text}
          </pre>
        )
      case 'hr':
        return <hr key={key} className="border-border" />
      default:
        return null
    }
  })
}

// ReleaseNotes renders release-note markdown from tokens, never from HTML:
// raw HTML and images are dropped and only http(s) and mailto links are live.
export function ReleaseNotes({ markdown }: { markdown: string }) {
  const cleaned = stripNoteComments(markdown)
  if (cleaned === '') return null
  return (
    <div className="space-y-1.5 text-xs text-muted-foreground">
      {blocks(lexer(cleaned, { gfm: true }), 'n')}
    </div>
  )
}
