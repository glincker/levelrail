import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { sanitizeDigits } from '../../lib/signInRequest'

const SLOTS = [0, 1] as const

// Two single-digit boxes that behave as one field: typing advances, backspace
// retreats, and a pasted number fills both.
export function MatchDigits({
  value,
  onChange,
  invalid,
  describedBy,
  autoFocus = true,
}: {
  value: string
  onChange: (next: string) => void
  invalid?: boolean
  describedBy?: string
  autoFocus?: boolean
}) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const refs = useRef<Array<HTMLInputElement | null>>([])
  useEffect(() => {
    if (autoFocus) {
      refs.current[0]?.focus()
    }
  }, [autoFocus])

  const setAt = (index: number, digits: string) => {
    if (digits === '') {
      onChange(value.slice(0, index))
      return
    }
    const next = sanitizeDigits(value.slice(0, index) + digits)
    onChange(next)
    refs.current[Math.min(next.length, 1)]?.focus()
  }

  return (
    <div
      role="group"
      aria-label={t('requests.matchGroup')}
      className="flex items-center gap-2"
    >
      {SLOTS.map((i) => (
        <input
          key={i}
          ref={(el) => {
            refs.current[i] = el
          }}
          type="text"
          inputMode="numeric"
          autoComplete="off"
          maxLength={2}
          value={value[i] ?? ''}
          aria-label={t('requests.matchDigit', { n: i + 1 })}
          aria-invalid={invalid ? true : undefined}
          aria-describedby={describedBy}
          className={cn(
            'size-14 rounded-lg border bg-background text-center font-mono text-2xl font-semibold transition-colors outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring',
            invalid ? 'border-destructive' : 'border-input',
          )}
          onChange={(e) => setAt(i, sanitizeDigits(e.target.value).slice(-1))}
          onPaste={(e) => {
            const pasted = sanitizeDigits(e.clipboardData.getData('text'))
            if (pasted === '') return
            e.preventDefault()
            onChange(pasted)
            refs.current[Math.min(pasted.length, 1)]?.focus()
          }}
          onKeyDown={(e) => {
            if (e.key === 'Backspace' && !value[i] && i > 0) {
              e.preventDefault()
              onChange(value.slice(0, i - 1))
              refs.current[i - 1]?.focus()
            } else if (e.key === 'ArrowLeft' && i > 0) {
              refs.current[i - 1]?.focus()
            } else if (e.key === 'ArrowRight' && i < 1) {
              refs.current[i + 1]?.focus()
            }
          }}
        />
      ))}
    </div>
  )
}
