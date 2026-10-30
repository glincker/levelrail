import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ShieldAlertGlassIcon, ShieldGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { advisoriesQueryOptions } from '../../queries/securityCenter'

// Renders nothing until a feed is configured, so a default install sees no card.
export function AdvisoriesCard() {
  const { t } = useTranslation('security')
  const { data, error } = useQuery(advisoriesQueryOptions())
  if (error) {
    return (
      <p className="text-sm text-destructive">{t('advisories.loadError')}</p>
    )
  }
  if (!data?.enabled) return null
  const signature = data.signature ?? 'unsigned'
  const Icon = data.matches.length > 0 ? ShieldAlertGlassIcon : ShieldGlassIcon
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3 space-y-0 pb-2">
        <CardTitle className="flex items-center gap-2 text-base">
          <Icon className="size-5" aria-hidden />
          {t('advisories.title')}
        </CardTitle>
        <Badge variant={signature === 'verified' ? 'success' : 'warning'}>
          {t('advisories.feed', {
            count: data.entries.length,
            signature: t(`advisories.${signature}`),
          })}
        </Badge>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        {data.error ? <p className="text-destructive">{data.error}</p> : null}
        {!data.error && data.matches.length === 0 ? (
          <p className="text-muted-foreground">{t('advisories.none')}</p>
        ) : null}
        {data.matches.length > 0 ? (
          <p className="font-medium">
            {t('advisories.summary', { count: data.matches.length })}
          </p>
        ) : null}
        <ul className="divide-y">
          {data.matches.map((m) => (
            <li
              key={`${m.app}:${m.advisory_id}`}
              className="flex flex-wrap items-baseline gap-x-3 gap-y-1 py-2"
            >
              <span className="font-medium">{m.app}</span>
              <Badge variant="outline">{m.severity}</Badge>
              <span>{m.advisory_id}</span>
              <span className="text-muted-foreground">{m.hint}</span>
              {m.reference ? (
                <a
                  className="text-primary underline"
                  href={m.reference}
                  target="_blank"
                  rel="noreferrer"
                >
                  {t('advisories.reference')}
                </a>
              ) : null}
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
