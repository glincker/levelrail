import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { COPY_FEEDBACK_MS } from '@/lib/copyFeedback'
import { cn } from '@/lib/utils'

export interface TerminalLine {
  kind: 'comment' | 'command'
  text: string
}

type CopyState = 'idle' | 'copied' | 'failed'

/** Copy with a timed "copied" state; the pending reset is cleared on unmount. */
export function useTerminalCopy(text: string) {
  const [state, setState] = useState<CopyState>('idle')
  const timer = useRef<number | undefined>(undefined)
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

export function TerminalCopyButton({
  text,
  label,
  variant = 'outline',
  disabled = false,
}: {
  text: string
  label?: string
  variant?: 'outline' | 'ghost'
  disabled?: boolean
}) {
  const { t } = useTranslation('common')
  const { state, copy } = useTerminalCopy(text)
  return (
    <>
      <Button
        type="button"
        size="xs"
        variant={variant}
        disabled={disabled}
        className="shrink-0 justify-center"
        onClick={() => void copy()}
      >
        {state === 'copied' ? (
          <CheckGlassIcon size="1.4em" aria-hidden="true" />
        ) : (
          <CopyGlassIcon size="1.4em" aria-hidden="true" />
        )}
        {state === 'copied'
          ? t('codeBlock.copied')
          : (label ?? t('codeBlock.copy'))}
      </Button>
      <span role="status" aria-live="polite" className="sr-only">
        {state === 'copied' ? t('codeBlock.copied') : null}
        {state === 'failed' ? t('codeBlock.copyFailed') : null}
      </span>
    </>
  )
}

// Tints through existing tokens only: flags and URLs read as info.
function Tinted({ text }: { text: string }) {
  const parts = text.split(/(\s+)/)
  const binaryAt = parts.findIndex((p) => !/^\s*$/.test(p))
  return (
    <>
      {parts.map((part, i) => {
        if (/^\s*$/.test(part)) {
          return part
        }
        const isBinary = i === binaryAt
        const accent = /^-{1,2}\w|^https?:\/\//.test(part)
        return (
          <span
            key={i}
            className={cn(
              isBinary && 'font-medium text-foreground',
              accent && 'text-tone-info',
            )}
          >
            {part}
          </span>
        )
      })}
    </>
  )
}

/** One terminal session: comment lines label the commands, one Copy copies the commands only. */
export function TerminalBlock({
  lines,
  label,
  prompt = '$',
  footer,
  className,
}: {
  lines: TerminalLine[]
  /** Names the scroll region for assistive tech. */
  label: string
  prompt?: string
  /** Actions or status shown under the lines, inside the surface. */
  footer?: ReactNode
  className?: string
}) {
  const commands = lines
    .filter((l) => l.kind === 'command')
    .map((l) => l.text)
    .join('\n')
  return (
    <div
      className={cn(
        'min-w-0 rounded-lg border border-border bg-muted/60',
        className,
      )}
    >
      <div className="flex items-start gap-2 p-2">
        <pre
          role="group"
          tabIndex={0}
          aria-label={label}
          className="min-w-0 flex-1 overflow-x-auto rounded-sm p-1 font-mono text-xs leading-5 whitespace-pre focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          <code>
            {lines.map((line, i) => (
              <span key={i} className="flex">
                {line.kind === 'command' ? (
                  <span
                    aria-hidden="true"
                    className="w-4 shrink-0 text-tone-success select-none"
                  >
                    {prompt}
                  </span>
                ) : null}
                <span
                  className={cn(
                    'min-w-0',
                    line.kind === 'comment' && 'text-muted-foreground',
                  )}
                >
                  {line.kind === 'comment' ? (
                    `# ${line.text}`
                  ) : (
                    <Tinted text={line.text} />
                  )}
                </span>
              </span>
            ))}
          </code>
        </pre>
        {commands ? <TerminalCopyButton text={commands} /> : null}
      </div>
      {footer ? (
        <div className="flex flex-wrap items-center gap-2 border-t border-border px-3 py-2">
          {footer}
        </div>
      ) : null}
    </div>
  )
}
