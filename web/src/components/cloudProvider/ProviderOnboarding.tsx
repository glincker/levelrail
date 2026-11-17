import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  ExternalLinkGlassIcon,
  PlugGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ProviderLinks } from './ProviderLinks'
import type { SectionProps } from './sections'

// One compact card for the not connected state: the sections it replaces would
// all be empty without a credential.
export function ProviderOnboarding({
  provider,
  labels,
  connection,
}: SectionProps) {
  const { t } = useTranslation('cloudProvider')
  const unlocks = [
    provider.features.includes('create') &&
      t('onboarding.unlock.create', { servers: labels.servers }),
    t('onboarding.unlock.manage', { servers: labels.servers }),
    provider.features.includes('capabilities') &&
      t('onboarding.unlock.permissions'),
    provider.features.includes('spend') && t('onboarding.unlock.spend'),
  ].filter((x): x is string => typeof x === 'string')
  return (
    <section
      aria-labelledby="cp-onboarding"
      className="space-y-3 rounded-lg border border-border p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="cp-onboarding" className="text-sm font-semibold">
          {t('onboarding.title', { name: labels.name })}
        </h2>
        <Button type="button" size="sm" onClick={connection.openConnect}>
          <PlugGlassIcon size="1.4em" aria-hidden="true" />
          {t('connection.connect')}
        </Button>
      </div>
      <div className="grid gap-x-8 gap-y-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <h3 className="text-xs font-medium text-muted-foreground">
            {t('onboarding.unlocksTitle')}
          </h3>
          <ul className="space-y-1 text-sm">
            {unlocks.map((u) => (
              <li key={u} className="flex items-start gap-2">
                <CircleCheckGlassIcon
                  className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                  aria-hidden="true"
                />
                {u}
              </li>
            ))}
          </ul>
        </div>
        <div className="space-y-1.5">
          <h3 className="text-xs font-medium text-muted-foreground">
            {t('onboarding.stepsTitle')}
          </h3>
          <ol className="list-decimal space-y-1 ps-5 text-sm marker:text-muted-foreground">
            <li>
              {provider.tokenUrl ? (
                <a
                  href={provider.tokenUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 underline underline-offset-2 hover:text-foreground"
                >
                  {t('onboarding.step.get')}
                  <ExternalLinkGlassIcon size="1.2em" aria-hidden="true" />
                </a>
              ) : (
                t('onboarding.step.get')
              )}
            </li>
            <li>{t('onboarding.step.paste')}</li>
            <li>{t('onboarding.step.verify')}</li>
          </ol>
          <p className="text-xs text-muted-foreground">{labels.tokenHelp}</p>
        </div>
      </div>
      <ProviderLinks provider={provider} labels={labels} />
    </section>
  )
}
