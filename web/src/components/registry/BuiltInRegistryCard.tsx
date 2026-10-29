import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ServerGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { useRegistryStatus } from '../../queries/registry'
import { RegistryStatusBadge } from '../RegistrySettingsCard'

export function BuiltInRegistryCard() {
  const { t } = useTranslation('settings')
  const { data } = useRegistryStatus()
  if (!data) return null
  const active = data.enabled
  return (
    <Card>
      <CardContent className="flex flex-wrap items-center gap-4">
        <span className="inline-flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <ServerGlassIcon aria-hidden="true" className="size-5.5" />
        </span>
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-sm font-medium">
              {active
                ? t('builtInRegistry.titleActive')
                : t('builtInRegistry.title')}
            </h2>
            {active && <RegistryStatusBadge status={data.status} />}
          </div>
          <p className="text-xs text-muted-foreground">
            {active && data.host
              ? t('builtInRegistry.activeBody', { host: data.host })
              : t('builtInRegistry.body')}
          </p>
        </div>
        <Button
          variant={active ? 'outline' : 'default'}
          size="sm"
          render={<Link to="/settings/registry" />}
        >
          {active ? t('builtInRegistry.manage') : t('builtInRegistry.setUp')}
        </Button>
      </CardContent>
    </Card>
  )
}
