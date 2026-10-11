import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

const COPIED_MS = 2000

export interface CopyFieldProps {
  value: string
  label: string
  mono?: boolean
  multiline?: boolean
  onCopied?: () => void
  className?: string
}

/** Read-only value with a copy button that falls back to selecting text. */
export function CopyField({
  value,
  label,
  mono = true,
  multiline = false,
  onCopied,
  className,
}: Readonly<CopyFieldProps>) {
  const { t } = useTranslation('traffic')
  const [state, setState] = useState<'idle' | 'copied' | 'manual'>('idle')
  const valueRef = useRef<HTMLSpanElement>(null)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => () => clearTimeout(timer.current), [])

  function reset() {
    clearTimeout(timer.current)
    timer.current = setTimeout(() => setState('idle'), COPIED_MS)
  }

  function selectValue() {
    const node = valueRef.current
    if (!node) return
    const range = document.createRange()
    range.selectNodeContents(node)
    const selection = window.getSelection()
    selection?.removeAllRanges()
    selection?.addRange(range)
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setState('copied')
      onCopied?.()
    } catch {
      selectValue()
      setState('manual')
    }
    reset()
  }

  const Tag = multiline ? 'pre' : 'code'
  return (
    <div className={cn('flex items-start gap-2', className)}>
      <div className="min-w-0 flex-1">
        <Tag
          aria-label={label}
          className={cn(
            'block rounded-md border border-border bg-muted/40 px-2.5 py-1.5 text-[13px]',
            mono && 'font-mono',
            multiline
              ? 'max-h-60 overflow-auto whitespace-pre-wrap break-all'
              : 'truncate',
          )}
        >
          <span ref={valueRef}>{value}</span>
        </Tag>
        <p className="sr-only" role="status" aria-live="polite">
          {state === 'copied' ? t('copy.copied') : ''}
        </p>
        {state === 'manual' ? (
          <p className="pt-1 text-xs text-muted-foreground">
            {t('copy.fallback')}
          </p>
        ) : null}
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        aria-label={t('copy.copyLabel', { label })}
        onClick={() => void copy()}
      >
        {state === 'copied' ? <CheckIcon /> : <CopyIcon />}
        {state === 'copied' ? t('copy.copied') : t('copy.copy')}
      </Button>
    </div>
  )
}
