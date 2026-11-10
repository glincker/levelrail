import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { ingressSettingsQueryOptions } from '../../queries/domains'

const STEPS = ['detect', 'review', 'deploy'] as const

/** Value summary for the aside: what the next steps do, plus a slot for the provider card. */
export function NewAppAside({
  setSlot,
}: {
  setSlot: (el: HTMLDivElement | null) => void
}) {
  const { t } = useTranslation('newApp')
  const { data: ingress } = useQuery(ingressSettingsQueryOptions())
  const domain = ingress?.primary_domain
  return (
    <>
      <section
        aria-labelledby="new-app-aside-title"
        className="space-y-3 rounded-container border border-border bg-card p-4 shadow-card"
      >
        <h2
          id="new-app-aside-title"
          className="text-sm font-semibold text-foreground"
        >
          {t('aside.title')}
        </h2>
        <ol className="space-y-2">
          {STEPS.map((s, i) => (
            <li key={s} className="flex items-start gap-3 text-sm">
              <span className="flex size-6 shrink-0 items-center justify-center rounded-full border border-border text-xs font-semibold text-muted-foreground">
                {i + 1}
              </span>
              <span className="text-muted-foreground">{t(`aside.${s}`)}</span>
            </li>
          ))}
        </ol>
        {domain ? (
          <p className="border-t border-border pt-3 text-sm text-muted-foreground">
            {t('aside.address', { domain })}
          </p>
        ) : null}
      </section>
      <div ref={setSlot} className="empty:hidden" />
    </>
  )
}
