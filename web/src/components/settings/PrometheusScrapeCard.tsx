import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ActivityGlassIcon, KeyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { CodeBlock, CommandBlock } from '@/components/ui/code-block'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useCreateToken } from '../../queries/tokens'
import {
  useRuntimeSettings,
  useSetRuntimeSetting,
} from '../../queries/runtimeSettings'

const SCRAPE_KEY = 'observability.metrics_scrape_enabled'

function scrapeJob(token: string): string {
  return [
    'scrape_configs:',
    '  - job_name: levelrail',
    '    metrics_path: /metrics',
    `    scheme: ${window.location.protocol.replace(':', '')}`,
    '    authorization:',
    `      credentials: ${token}`,
    '    static_configs:',
    `      - targets: ['${window.location.host}']`,
  ].join('\n')
}

/** PrometheusScrapeCard toggles the scrape endpoint and mints a read token shown once. */
export function PrometheusScrapeCard() {
  const { t } = useTranslation('runtimeSettings')
  const settings = useRuntimeSettings()
  const setSetting = useSetRuntimeSetting()
  const create = useCreateToken()
  const [token, setToken] = useState('')
  const current = settings.data?.settings.find((s) => s.key === SCRAPE_KEY)
  if (!current) return null
  const enabled = current.value === true

  function createToken() {
    create.mutate(
      { name: 'prometheus-scrape', abilities: ['read'] },
      {
        onSuccess: (res) => setToken(res.token),
        onError: (err) =>
          toast.add({
            title: t('prometheus.error', { message: err.message }),
            type: 'error',
          }),
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ActivityGlassIcon className="size-5 text-muted-foreground" />
          {t('prometheus.title')}
        </CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('prometheus.description')}
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center gap-3">
          <Switch
            checked={enabled}
            disabled={setSetting.isPending}
            aria-label={t('prometheus.toggle')}
            onCheckedChange={(next) =>
              setSetting.mutate(
                { key: SCRAPE_KEY, value: next },
                {
                  onError: (err) =>
                    toast.add({
                      title: t('prometheus.saveError', {
                        message: err.message,
                      }),
                      type: 'error',
                    }),
                },
              )
            }
          />
          <span className="text-sm text-foreground">
            {t('prometheus.toggle')}
          </span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={create.isPending || token !== ''}
            onClick={createToken}
          >
            <KeyGlassIcon className="size-5" aria-hidden="true" />
            {create.isPending
              ? t('prometheus.creating')
              : t('prometheus.create')}
          </Button>
        </div>
        {token ? (
          <div className="space-y-2">
            <p className="text-sm text-tone-warning" role="status">
              {t('prometheus.tokenOnce')}
            </p>
            <CommandBlock
              code={token}
              label={t('prometheus.tokenLabel')}
              prompt={false}
              secret
            />
            <CodeBlock
              code={scrapeJob(token)}
              language="yaml"
              label={t('prometheus.configLabel')}
              secret
            />
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => {
                setToken('')
                create.reset()
              }}
            >
              {t('prometheus.dismiss')}
            </Button>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
