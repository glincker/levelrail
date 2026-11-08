import { type UiIcon } from '@thesvg/ui-react'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import {
  STATUS,
  TONE,
  TONE_BADGE_VARIANT,
  type StatusState,
  type Tone,
} from '@/components/kit/tone'
import { StatusShape } from '@/components/kit/StatusShape'
import { cn } from '@/lib/utils'

type BadgeVariant = VariantProps<typeof badgeVariants>['variant']

export function StatusBadge({
  variant,
  tone,
  label,
  icon: StatusIcon,
  state,
  form = 'pill',
}: Readonly<{
  variant?: BadgeVariant
  tone?: Tone
  label: string
  icon?: UiIcon
  /** A known state picks tone, shape and ping from the status map. */
  state?: StatusState
  /** dot is bare shape plus text; pill is the tinted capsule badge. */
  form?: 'dot' | 'pill'
}>) {
  const resolvedTone = state ? STATUS[state].tone : tone
  if (state && form === 'dot') {
    return (
      <span
        className={cn(
          'inline-flex items-center gap-1.5 text-xs font-medium whitespace-nowrap',
          TONE[STATUS[state].tone].text,
        )}
      >
        <StatusShape state={state} />
        {label}
      </span>
    )
  }
  const resolved = resolvedTone ? TONE_BADGE_VARIANT[resolvedTone] : variant
  return (
    <Badge variant={resolved} className="shrink-0">
      {state ? (
        <StatusShape state={state} />
      ) : StatusIcon ? (
        <StatusIcon className="size-3.5" aria-hidden="true" />
      ) : null}
      {label}
    </Badge>
  )
}
