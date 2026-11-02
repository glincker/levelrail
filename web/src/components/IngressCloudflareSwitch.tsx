import { useTranslation } from 'react-i18next'
import {
  useIngressCloudflare,
  useSetIngressCloudflare,
} from '../queries/ingressCloudflare'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'

// Applies immediately (stored runtime setting), so it sits outside the
// card's Save form.
export function IngressCloudflareSwitch() {
  const { t } = useTranslation('ingressProtection')
  const { data } = useIngressCloudflare()
  const setPreset = useSetIngressCloudflare()
  if (!data) return null
  const on = data.preset === 'cloudflare'

  return (
    <div
      id="cloudflare-client-ip"
      className="scroll-mt-20 rounded-md border border-border p-3"
    >
      <Field orientation="horizontal">
        <Switch
          id="cloudflare-client-ip-switch"
          checked={on}
          onCheckedChange={(next) => {
            setPreset.mutate(next ? 'cloudflare' : 'none', {
              onSuccess: () => {
                toast.add({
                  title: next
                    ? t('cloudflare.saved')
                    : t('cloudflare.savedOff'),
                  type: 'success',
                })
              },
              onError: (err) => {
                toast.add({
                  title: t('cloudflare.failed'),
                  description: err.message,
                  type: 'error',
                })
              },
            })
          }}
        />
        <FieldLabel htmlFor="cloudflare-client-ip-switch">
          {t('cloudflare.label')}
        </FieldLabel>
      </Field>
      <FieldDescription className="mt-1">
        {t('cloudflare.helper')}
      </FieldDescription>
      <FieldDescription className="mt-1">
        {t('cloudflare.ranges', {
          count: data.ranges,
          source: t(`cloudflare.source.${data.ranges_source}`),
          interval: data.refresh_interval,
        })}{' '}
        {data.last_refresh
          ? t('cloudflare.lastRefresh', {
              when: new Date(data.last_refresh).toLocaleString(),
            })
          : t('cloudflare.neverRefreshed')}
        {data.last_error
          ? ` ${t('cloudflare.lastError', { error: data.last_error })}`
          : ''}
      </FieldDescription>
    </div>
  )
}
