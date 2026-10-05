import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { GlobeIcon } from '@phosphor-icons/react/dist/ssr'
import {
  ingressSettingsQueryOptions,
  useUpdateIngressSettings,
} from '../queries/domains'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'

/** FallbackDomainsCard toggles the automatic sslip.io hostname every app without a domain gets. */
export function FallbackDomainsCard() {
  const { t } = useTranslation('https')
  const { data: settings } = useQuery(ingressSettingsQueryOptions())
  const update = useUpdateIngressSettings()
  if (!settings) return null

  const enabled = settings.fallback_domains_enabled ?? true
  const host = settings.public_host
  const source = settings.public_host_source ?? 'none'
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GlobeIcon className="size-4" />
          {t('fallback.title')}
        </CardTitle>
        <CardDescription>{t('fallback.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-center gap-3">
          <Switch
            id="fallback-domains-toggle"
            checked={enabled}
            disabled={update.isPending}
            onCheckedChange={(checked) =>
              update.mutate(
                { ...settings, fallback_domains_enabled: checked },
                {
                  onSuccess: () =>
                    toast.add({ title: t('fallback.saved'), type: 'success' }),
                  onError: () =>
                    toast.add({
                      title: t('fallback.saveFailed'),
                      type: 'error',
                    }),
                },
              )
            }
          />
          <Label htmlFor="fallback-domains-toggle">
            {t('fallback.toggleLabel')}
          </Label>
        </div>
        <p className="text-xs text-muted-foreground">
          {host
            ? t('fallback.publicHost', {
                host,
                source: t(`fallback.source.${source}`),
              })
            : t('fallback.noPublicHost')}
        </p>
      </CardContent>
    </Card>
  )
}
