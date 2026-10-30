import { useCallback, useEffect, useRef, useState } from 'react'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  EyeGlassIcon,
  EyeOffGlassIcon,
} from '@thesvg/ui-react'
import { ListGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { COPY_FEEDBACK_MS } from '@/lib/copyFeedback'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'

type CopyState = 'idle' | 'copied' | 'failed'

const MAX_HEIGHT = {
  none: '',
  sm: 'max-h-40',
  md: 'max-h-72',
  lg: 'max-h-96',
} as const

export type CodeBlockMaxHeight = keyof typeof MAX_HEIGHT

const PROMPT = '$ '

function useCopyFeedback(text: string) {
  const [state, setState] = useState<CopyState>('idle')
  const timer = useRef<number | undefined>(undefined)

  // A pending reset must not fire after unmount.
  useEffect(
    () => () => {
      window.clearTimeout(timer.current)
    },
    [],
  )

  const copy = useCallback(async () => {
    let next: CopyState = 'copied'
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      next = 'failed'
    }
    setState(next)
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      setState('idle')
    }, COPY_FEEDBACK_MS)
  }, [text])

  return { state, copy }
}

// Keeps whitespace so a masked block has the same shape as the real one.
function maskSecret(text: string): string {
  return text.replace(/\S/g, '•')
}

function stripPrompt(code: string): string {
  return code
    .split('\n')
    .map((line) => (line.startsWith(PROMPT) ? line.slice(PROMPT.length) : line))
    .join('\n')
}

interface SharedProps {
  code: string
  /** Names the scroll region for assistive tech. */
  label?: string
  /** Masks every visible character until the reader reveals it. Copy still yields the real value. */
  secret?: boolean
  className?: string
}

function CopyButton({
  state,
  onCopy,
  size,
}: {
  state: CopyState
  onCopy: () => void
  size: 'xs' | 'sm'
}) {
  const { t } = useTranslation('common')
  return (
    <>
      <Button
        type="button"
        size={size}
        variant="outline"
        className="min-w-20 shrink-0 justify-center"
        onClick={onCopy}
      >
        {state === 'copied' ? (
          <CheckGlassIcon size="1.4em" aria-hidden="true" />
        ) : (
          <CopyGlassIcon size="1.4em" aria-hidden="true" />
        )}
        {state === 'copied' ? t('codeBlock.copied') : t('codeBlock.copy')}
      </Button>
      <span role="status" aria-live="polite" className="sr-only">
        {state === 'copied' ? t('codeBlock.copied') : null}
        {state === 'failed' ? t('codeBlock.copyFailed') : null}
      </span>
    </>
  )
}

function RevealButton({
  revealed,
  onToggle,
  size,
}: {
  revealed: boolean
  onToggle: () => void
  size: 'xs' | 'sm'
}) {
  const { t } = useTranslation('common')
  return (
    <Button
      type="button"
      size={size}
      variant="ghost"
      aria-pressed={revealed}
      onClick={onToggle}
    >
      {revealed ? (
        <EyeOffGlassIcon size="1.4em" aria-hidden="true" />
      ) : (
        <EyeGlassIcon size="1.4em" aria-hidden="true" />
      )}
      {revealed ? t('codeBlock.hide') : t('codeBlock.reveal')}
    </Button>
  )
}

export interface CodeBlockProps extends SharedProps {
  /** Language chip, for example "yaml" or "bash". */
  language?: string
  /** Start wrapped; default is horizontal scroll. */
  wrap?: boolean
  /** Show the wrap toggle in the header. */
  wrapToggle?: boolean
  lineNumbers?: boolean
  /** Render a non-copied "$" before each line. */
  prompt?: boolean
  maxHeight?: CodeBlockMaxHeight
}

/** Shared multi-line code, config and output block with copy, wrap and secret masking. */
export function CodeBlock({
  code,
  language,
  label,
  wrap = false,
  wrapToggle = true,
  lineNumbers = false,
  prompt = false,
  maxHeight = 'md',
  secret = false,
  className,
}: CodeBlockProps) {
  const { t } = useTranslation('common')
  const [wrapped, setWrapped] = useState(wrap)
  const [revealed, setRevealed] = useState(false)
  const copyText = prompt ? stripPrompt(code) : code
  const { state, copy } = useCopyFeedback(copyText)
  const shown = secret && !revealed ? maskSecret(copyText) : copyText
  const lines = shown.split('\n')
  const decorated = lineNumbers || prompt
  const chip = language ?? label

  return (
    <div
      className={cn(
        'overflow-hidden rounded-lg border border-border bg-muted/40',
        className,
      )}
    >
      <div className="flex min-h-9 items-center justify-between gap-2 border-b border-border px-2 py-1">
        <span className="truncate font-mono text-xs text-muted-foreground">
          {chip}
        </span>
        <div className="flex shrink-0 items-center gap-1">
          {secret ? (
            <RevealButton
              revealed={revealed}
              size="xs"
              onToggle={() => {
                setRevealed((v) => !v)
              }}
            />
          ) : null}
          {wrapToggle ? (
            <Button
              type="button"
              size="xs"
              variant="ghost"
              aria-pressed={wrapped}
              onClick={() => {
                setWrapped((v) => !v)
              }}
            >
              <ListGlassIcon size="1.4em" aria-hidden="true" />
              {t('codeBlock.wrap')}
            </Button>
          ) : null}
          <CopyButton state={state} size="xs" onCopy={() => void copy()} />
        </div>
      </div>
      <pre
        role="group"
        tabIndex={0}
        aria-label={label ?? t('codeBlock.region')}
        className={cn(
          'overflow-auto p-3 font-mono text-xs leading-5 text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset',
          MAX_HEIGHT[maxHeight],
          wrapped ? 'break-words whitespace-pre-wrap' : 'whitespace-pre',
        )}
      >
        <code>
          {decorated
            ? lines.map((line, i) => (
                <span key={i} className="flex">
                  {lineNumbers ? (
                    <span
                      aria-hidden="true"
                      className="w-8 shrink-0 pr-3 text-right text-muted-foreground tabular-nums select-none"
                    >
                      {i + 1}
                    </span>
                  ) : null}
                  {prompt ? (
                    <span
                      aria-hidden="true"
                      className="shrink-0 text-muted-foreground select-none"
                    >
                      {PROMPT}
                    </span>
                  ) : null}
                  <span className="min-w-0 flex-1">{line}</span>
                </span>
              ))
            : shown}
        </code>
      </pre>
    </div>
  )
}

export interface CommandBlockProps extends SharedProps {
  /** Show a non-copied "$" before the command. Default on. */
  prompt?: boolean
  wrap?: boolean
}

/** One-line copyable shell command: a compact row with the copy button inline. */
export function CommandBlock({
  code,
  label,
  prompt = true,
  wrap = false,
  secret = false,
  className,
}: CommandBlockProps) {
  const { t } = useTranslation('common')
  const [revealed, setRevealed] = useState(false)
  const { state, copy } = useCopyFeedback(code)
  const shown = secret && !revealed ? maskSecret(code) : code

  return (
    <div
      className={cn(
        'flex items-center gap-2 rounded-lg border border-border bg-muted/40 p-2',
        className,
      )}
    >
      <pre
        role="group"
        tabIndex={0}
        aria-label={label ?? t('codeBlock.commandRegion')}
        className={cn(
          'min-w-0 flex-1 overflow-x-auto font-mono text-xs leading-5 text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
          wrap ? 'break-all whitespace-pre-wrap' : 'whitespace-pre',
        )}
      >
        {prompt ? (
          <span
            aria-hidden="true"
            className="text-muted-foreground select-none"
          >
            {PROMPT}
          </span>
        ) : null}
        <code>{shown}</code>
      </pre>
      {secret ? (
        <RevealButton
          revealed={revealed}
          size="sm"
          onToggle={() => {
            setRevealed((v) => !v)
          }}
        />
      ) : null}
      <CopyButton state={state} size="sm" onCopy={() => void copy()} />
    </div>
  )
}
