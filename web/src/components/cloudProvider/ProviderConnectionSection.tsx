import { useTranslation } from 'react-i18next'
import { PlugGlassIcon, SettingsGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { RelativeTime } from '../kit/RelativeTime'
import type { SectionProps } from './sections'

// Rail summary shown once a credential exists. Status itself lives in the
// header pill, so this card only says who connected it and how to change it.
export function ProviderConnectionSection({
  resource,
  connection,
}: SectionProps) {
  const { t } = useTranslation('cloudProvider')
  const connected = resource.has_token
  return (
    <section
      aria-labelledby="cp-connection"
      className="space-y-2 rounded-lg border border-border p-3"
    >
      <h2 id="cp-connection" className="text-sm font-semibold">
        {t('connection.title')}
      </h2>
      {connected ? (
        <dl className="space-y-0.5 text-xs text-muted-foreground">
          {resource.label ? (
            <div className="flex gap-1.5">
              <dt>{t('connection.credential')}</dt>
              <dd className="min-w-0 truncate font-medium text-foreground">
                {resource.label}
              </dd>
            </div>
          ) : null}
          {resource.connected_by ? (
            <div className="flex gap-1.5">
              <dt>{t('connection.by')}</dt>
              <dd className="min-w-0 truncate text-foreground">
                {resource.connected_by}
              </dd>
            </div>
          ) : null}
          {resource.last_tested_at ? (
            <div className="flex gap-1.5">
              <dt>{t('connection.tested')}</dt>
              <dd className="text-foreground">
                <RelativeTime at={resource.last_tested_at} />
              </dd>
            </div>
          ) : null}
        </dl>
      ) : (
        <p className="text-xs text-muted-foreground">
          {t('connection.manualHint')}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={connection.openConnect}
        >
          <SettingsGlassIcon size="1.4em" aria-hidden="true" />
          {connected ? t('connection.update') : t('connection.connect')}
        </Button>
        {connected ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={connection.askDisconnect}
          >
            <PlugGlassIcon size="1.4em" aria-hidden="true" />
            {t('connection.disconnect')}
          </Button>
        ) : null}
      </div>
    </section>
  )
}
