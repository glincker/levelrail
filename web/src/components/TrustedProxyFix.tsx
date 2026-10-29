import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon, WrenchGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../hooks/useIsRoot'
import {
  edgeSettingsQueryOptions,
  useEdgeSettingsActions,
} from '../queries/ingressEdge'

export function TrustedProxyFix() {
  const { t } = useTranslation('domains')
  const isRoot = useIsRoot()
  const { data } = useQuery(edgeSettingsQueryOptions())
  const { fixProxy, recheck } = useEdgeSettingsActions()
  if (!data || !data.needs_trusted_proxies) return null

  const det = data.detected
  const canFix = isRoot && !data.env_locked.trusted_proxies && det.suggested
  const busy = fixProxy.isPending || recheck.isPending

  return (
    <Alert variant="destructive">
      <WrenchGlassIcon className="size-5" aria-hidden="true" />
      <AlertTitle>
        {t('edge.fix.title', { proxy: t(`edge.proxy.${det.proxy}`) })}
      </AlertTitle>
      <AlertDescription className="space-y-3">
        <p>{t('edge.fix.body')}</p>
        {det.ranges_rotate ? <p>{t('edge.fix.rangesRotate')}</p> : null}
        {!det.suggested ? <p>{t('edge.fix.manual')}</p> : null}
        <div className="flex flex-wrap gap-2">
          {canFix ? (
            <Button
              type="button"
              size="sm"
              disabled={busy}
              onClick={() => {
                fixProxy.mutate(undefined, {
                  onSuccess: () => {
                    toast.add({ title: t('edge.fix.done'), type: 'success' })
                  },
                  onError: (e) => {
                    toast.add({ title: e.message, type: 'error' })
                  },
                })
              }}
            >
              {t('edge.fix.apply', { value: det.suggested })}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => {
              recheck.mutate(undefined, {
                onError: (e) => {
                  toast.add({ title: e.message, type: 'error' })
                },
              })
            }}
          >
            <RefreshCwGlassIcon className="size-5" aria-hidden="true" />
            {t('edge.fix.recheck')}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  )
}
