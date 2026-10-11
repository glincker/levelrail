import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { useResetPolicy } from '../../queries/domainPolicies'
import type { DomainPolicies } from '../../queries/domainPolicyTypes'
import { FieldMessage, Section } from './PolicyShared'

const KINDS = ['headers', 'forwarders', 'geo', 'cache', 'redirects'] as const

export function DangerZoneTab({
  app,
  domain,
  policies,
}: {
  app: string
  domain: string
  policies: DomainPolicies
}) {
  const { t } = useTranslation('domainPolicies')
  const reset = useResetPolicy(app, domain)
  const [confirming, setConfirming] = useState<string | null>(null)

  return (
    <Section title={t('danger.title')} description={t('danger.help')}>
      <ul className="space-y-2">
        {KINDS.map((k) => {
          const configured = Boolean(policies.updated_at[k])
          return (
            <li
              key={k}
              className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-destructive/30 p-3"
            >
              <div>
                <p className="text-sm font-medium">{t(`tabs.${k}`)}</p>
                <p className="text-xs text-muted-foreground">
                  {configured ? t('danger.configured') : t('danger.default')}
                </p>
              </div>
              {confirming === k ? (
                <div className="flex gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="destructive"
                    disabled={reset.isPending}
                    onClick={() =>
                      reset.mutate(k, { onSettled: () => setConfirming(null) })
                    }
                  >
                    {t('danger.confirm')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => setConfirming(null)}
                  >
                    {t('danger.cancel')}
                  </Button>
                </div>
              ) : (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={!configured}
                  onClick={() => setConfirming(k)}
                >
                  <ArrowCounterClockwiseIcon />
                  {t('danger.reset')}
                </Button>
              )}
            </li>
          )
        })}
      </ul>
      <FieldMessage message={reset.error?.message} />
    </Section>
  )
}
