import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import { Badge, type badgeVariants } from '@/components/ui/badge'

export function StatusBadge({
  variant,
  label,
  icon: StatusIcon,
}: Readonly<{
  variant: VariantProps<typeof badgeVariants>['variant']
  label: string
  icon: Icon
}>) {
  return (
    <Badge variant={variant} className="shrink-0">
      <StatusIcon className="size-3" />
      {label}
    </Badge>
  )
}
