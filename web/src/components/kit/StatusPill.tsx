import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { TONE, type Tone } from './tone'

export interface StatusPillProps {
  tone: Tone
  label: string
  live?: boolean
  icon?: ReactNode
  size?: 'sm' | 'md'
  title?: string
}

export function StatusPill({
  tone,
  label,
  live,
  icon,
  size = 'md',
  title,
}: StatusPillProps) {
  const t = TONE[tone]
  return (
    <span
      title={title}
      data-tone={tone}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border font-medium whitespace-nowrap',
        t.soft,
        t.border,
        t.text,
        size === 'sm' ? 'h-5 px-2 text-[11px]' : 'h-6 px-2.5 text-xs',
      )}
    >
      {icon ?? (
        <span className="relative flex size-1.5 shrink-0" aria-hidden="true">
          {live && (
            <span
              data-testid="pill-pulse"
              className={cn(
                'kit-pulse-ring absolute inset-0 rounded-full',
                t.solid,
              )}
            />
          )}
          <span className={cn('relative size-1.5 rounded-full', t.solid)} />
        </span>
      )}
      {label}
    </span>
  )
}
