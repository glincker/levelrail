import type { Icon } from '@phosphor-icons/react'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import { TONE_BADGE_VARIANT, type Tone } from '@/components/kit/tone'

type BadgeVariant = VariantProps<typeof badgeVariants>['variant']

export function StatusBadge({
  variant,
  tone,
  label,
  icon: StatusIcon,
}: Readonly<{
  variant?: BadgeVariant
  tone?: Tone
  label: string
  icon: Icon
}>) {
  const resolved = tone ? TONE_BADGE_VARIANT[tone] : variant
  return (
    <Badge variant={resolved} className="shrink-0">
      <StatusIcon className="size-3" aria-hidden="true" />
      {label}
    </Badge>
  )
}
