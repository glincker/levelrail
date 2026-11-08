import type { ReactNode } from 'react'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { SettingRows } from '@/components/ui/setting-row'
import { cn } from '@/lib/utils'

export interface SettingsSectionProps {
  title: ReactNode
  description?: ReactNode
  /** Right side of the header: a SaveStatus, a reset button, a menu. */
  actions?: ReactNode
  /** Anchor target for related links. */
  id?: string
  className?: string
  /** SettingRow children; they are stacked with dividers. */
  children: ReactNode
}

/** SettingsSection is a compact card of SettingRows; padding comes from the density tokens via Card. */
export function SettingsSection({
  title,
  description,
  actions,
  id,
  className,
  children,
}: SettingsSectionProps) {
  return (
    <Card
      id={id}
      className={cn(
        'scroll-mt-20 look-glass:rounded-(--glinr-radius-group)',
        className,
      )}
    >
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
        {actions ? (
          <CardAction className="flex items-center gap-2">{actions}</CardAction>
        ) : null}
      </CardHeader>
      <CardContent>
        <SettingRows>{children}</SettingRows>
      </CardContent>
    </Card>
  )
}
