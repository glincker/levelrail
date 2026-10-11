import { useTranslation } from 'react-i18next'
import { ArrowRightIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import type { PreviewHops } from '../../queries/domainPolicyTypes'

function statusVariant(status: number) {
  if (status >= 300 && status < 400) return 'warning' as const
  if (status >= 200 && status < 300) return 'success' as const
  if (status === 0) return 'muted' as const
  return 'destructive' as const
}

// HopsStrip renders "How a request is handled": each sample URL and the
// chain of responses it gets, status code first.
export function HopsStrip({ samples }: { samples: PreviewHops[] }) {
  const { t } = useTranslation('domainPolicies')
  return (
    <section aria-label={t('redirects.howTitle')} className="space-y-2">
      <h3 className="text-sm font-semibold">{t('redirects.howTitle')}</h3>
      <ul className="space-y-2">
        {samples.map((s) => (
          <li
            key={s.url}
            className="rounded-md border border-border p-2 text-xs"
          >
            <p className="font-mono text-foreground">{s.url}</p>
            <ol className="mt-1 flex flex-wrap items-center gap-1">
              {s.hops.map((h, i) => (
                <li key={`${h.url}-${i}`} className="flex items-center gap-1">
                  {i > 0 ? (
                    <ArrowRightIcon
                      className="size-3 text-muted-foreground"
                      aria-hidden="true"
                    />
                  ) : null}
                  <Badge variant={statusVariant(h.status)}>
                    {h.status === 0 ? t('redirects.noResponse') : h.status}
                  </Badge>
                  <span className="text-muted-foreground">{h.note}</span>
                  {h.location ? (
                    <span className="font-mono">{h.location}</span>
                  ) : null}
                </li>
              ))}
            </ol>
            {s.error ? (
              <p
                role="alert"
                className="mt-1 flex items-center gap-1 text-destructive"
              >
                <WarningIcon className="size-3" aria-hidden="true" />
                {s.error}
              </p>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  )
}
