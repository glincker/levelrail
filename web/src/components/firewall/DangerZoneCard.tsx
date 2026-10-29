import type { ReactNode } from 'react'
import { OctagonAlertGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { TONE } from '@/components/kit/tone'
import { cn } from '@/lib/utils'

/** DangerZoneCard groups switches that change how the whole instance behaves; rows go inside as SettingRows. */
export function DangerZoneCard({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <Card className="ring-tone-danger-border">
      <CardHeader>
        <CardTitle className={cn('flex items-center gap-2', TONE.danger.text)}>
          <OctagonAlertGlassIcon
            aria-hidden="true"
            className="size-4.5 text-tone-danger"
          />
          {title}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  )
}
