import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useIngressDns01, useSetIngressDns01 } from '../queries/ingressDns01'
import { PLATFORM_ANCHORS } from '../lib/platformSettingsIndex'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'

// Applies immediately (stored runtime setting), so it sits outside the
// card's Save form.
export function IngressDns01Switch() {
  const { t } = useTranslation('dns01')
  const { data } = useIngressDns01()
  const setDns01 = useSetIngressDns01()
  if (!data) return null

  // An already-on switch stays usable so it can always be turned off.
  const blocked = !data.provider_connected && !data.enabled

  return (
    <div
      id={PLATFORM_ANCHORS.dns01}
      className="scroll-mt-20 rounded-md border border-border p-3"
    >
      <Field orientation="horizontal">
        <Switch
          id="dns01-all-hosts"
          checked={data.enabled}
          disabled={blocked}
          onCheckedChange={(next) => {
            setDns01.mutate(next, {
              onSuccess: (updated) => {
                toast.add({
                  title: updated.enabled ? t('saved') : t('savedOff'),
                  type: 'success',
                })
              },
              onError: (err) => {
                toast.add({
                  title: next ? t('enableFailed') : t('disableFailed'),
                  description: err.message,
                  type: 'error',
                })
              },
            })
          }}
        />
        <FieldLabel htmlFor="dns01-all-hosts">{t('label')}</FieldLabel>
      </Field>
      <FieldDescription className="mt-1">
        {t('helper')}
        {blocked ? (
          <>
            {' '}
            {t('noProvider')}{' '}
            <Link
              to="/settings/platform/dns-providers"
              className="underline underline-offset-4"
            >
              {t('connectProvider')}
            </Link>
          </>
        ) : null}
      </FieldDescription>
    </div>
  )
}
