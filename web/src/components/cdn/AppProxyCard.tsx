import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { useAppCdn, useSetAppCdn } from '../../queries/appCdn'

/** AppProxyCard turns Cloudflare proxying on or off for one app's new DNS records. */
export function AppProxyCard({ app }: Readonly<{ app: string }>) {
  const { t } = useTranslation('cdn')
  const { data } = useAppCdn(app)
  const save = useSetAppCdn(app)
  if (!data) return null
  const following = data.proxied === null
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>{t('proxy.title')}</CardTitle>
        <Switch
          aria-label={t('proxy.title')}
          checked={data.effective}
          disabled={save.isPending}
          onCheckedChange={(on) => save.mutate(on)}
        />
      </CardHeader>
      <CardContent className="space-y-2 text-sm text-muted-foreground">
        <p>{t('proxy.body')}</p>
        <p>
          {following
            ? t('proxy.following', {
                state: data.global_proxied ? t('proxy.on') : t('proxy.off'),
              })
            : t('proxy.override')}
        </p>
        {following ? null : (
          <Button
            size="sm"
            variant="outline"
            writeAction
            disabled={save.isPending}
            onClick={() => save.mutate(null)}
          >
            {t('proxy.reset')}
          </Button>
        )}
        {save.isError ? (
          <p className="text-tone-danger">{save.error.message}</p>
        ) : null}
      </CardContent>
    </Card>
  )
}
