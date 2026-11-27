import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatMoney } from '../../lib/usageFormat'
import { useNodeListOptional } from '../../queries/nodes'
import type { CostExploration, CostServer } from '../../types/costExplorer'

function ServerBlock({
  server,
  name,
  currency,
}: {
  server: CostServer
  name: string
  currency: string
}) {
  const { t } = useTranslation('costs')
  const price = server.monthly_price
  const apps = server.apps ?? []
  const hasSaving = apps.some((a) => a.monthly_saving !== null)
  return (
    <section aria-label={name} className="space-y-1">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="text-sm font-medium">{name}</h3>
        <span className="text-sm text-muted-foreground tabular-nums">
          {formatMoney(price ?? 0, currency)}
        </span>
      </div>
      <table className="w-full text-sm">
        <caption className="sr-only">{t('servers.caption', { name })}</caption>
        <thead className="sr-only">
          <tr>
            <th scope="col">{t('servers.app')}</th>
            <th scope="col">{t('servers.share')}</th>
            {hasSaving ? <th scope="col">{t('servers.saving')}</th> : null}
          </tr>
        </thead>
        <tbody className="divide-y">
          {apps.map((a) => (
            <tr key={`${a.kind}-${a.name}`}>
              <td className="py-1.5">
                {a.kind === 'app' ? (
                  <Link
                    to="/apps/$name"
                    params={{ name: a.name }}
                    className="hover:underline"
                  >
                    {a.name}
                  </Link>
                ) : (
                  a.name
                )}
              </td>
              <td className="py-1.5 text-right tabular-nums">
                {a.share === null
                  ? null
                  : `${formatMoney(a.share, currency)}${
                      a.share_pct === null
                        ? ''
                        : ` (${a.share_pct.toFixed(0)}%)`
                    }`}
              </td>
              {hasSaving ? (
                <td className="py-1.5 pl-4 text-right text-muted-foreground tabular-nums">
                  {a.monthly_saving === null
                    ? null
                    : t('servers.savingValue', {
                        amount: formatMoney(a.monthly_saving, currency),
                      })}
                </td>
              ) : null}
            </tr>
          ))}
          {server.idle_share !== null ? (
            <tr data-testid="server-idle-row">
              <td className="py-1.5 italic text-muted-foreground">
                {t('servers.idle')}
              </td>
              <td className="py-1.5 text-right tabular-nums">
                {formatMoney(server.idle_share, currency)}
              </td>
              {hasSaving ? <td /> : null}
            </tr>
          ) : null}
        </tbody>
      </table>
    </section>
  )
}

/** Per-server bill split: one block per priced server, nothing for unpriced nodes. */
export function CostServers({ data }: { data: CostExploration }) {
  const { t } = useTranslation('costs')
  const { data: nodes } = useNodeListOptional()
  const priced = (data.servers ?? []).filter((s) => s.monthly_price !== null)
  if (priced.length === 0) return null
  const nameOf = (id: string) => nodes?.find((n) => n.id === id)?.name ?? id
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('servers.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('servers.description')}
        </p>
      </CardHeader>
      <CardContent className="grid gap-6 lg:grid-cols-2">
        {priced.map((s) => (
          <ServerBlock
            key={s.node_id}
            server={s}
            name={nameOf(s.node_id)}
            currency={data.currency}
          />
        ))}
      </CardContent>
    </Card>
  )
}
