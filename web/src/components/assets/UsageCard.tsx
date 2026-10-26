import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { useAssetUsage } from '../../queries/assets'
import { formatAssetSize, formatUsd, sizeLabel } from '../../lib/assetsFormat'
import type { CostLine } from '../../types/assets'

function parseEgress(raw: string): number {
  const n = Number(raw)
  return Number.isFinite(n) && n >= 0 ? n : 0
}

function CostRow({
  label,
  line,
  strong,
}: Readonly<{ label: string; line: CostLine; strong?: boolean }>) {
  const { t } = useTranslation('assets')
  return (
    <tr className={strong ? 'font-medium' : undefined}>
      <th scope="row" className="py-0.5 pr-2 text-left font-normal">
        {label}
        {line.free_egress_gb > 0 ? (
          <span className="block text-xs text-muted-foreground">
            {t('usage.freeEgress', { gb: line.free_egress_gb.toFixed(1) })}
          </span>
        ) : null}
      </th>
      <td className="px-2 text-right tabular-nums">
        {formatUsd(line.storage_usd)}
      </td>
      <td className="px-2 text-right tabular-nums">
        {formatUsd(line.egress_usd)}
      </td>
      <td className="pl-2 text-right tabular-nums">
        {formatUsd(line.total_usd)}
      </td>
    </tr>
  )
}

export function UsageCard({ bucketId }: Readonly<{ bucketId: string }>) {
  const { t } = useTranslation('assets')
  const [draft, setDraft] = useState('0')
  const [egress, setEgress] = useState(0)
  const { data, isPending, isError } = useAssetUsage(bucketId, egress)
  const presetName = (preset: string) => t(`preset.${preset}`, preset)

  function apply(e: FormEvent) {
    e.preventDefault()
    setEgress(parseEgress(draft))
  }

  return (
    <section
      aria-labelledby="asset-usage-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <h2 id="asset-usage-title" className="text-sm font-medium">
          {t('usage.title')}
        </h2>
        <Badge variant="info">{t('usage.estimated')}</Badge>
      </div>
      {isPending ? (
        <Skeleton className="h-16 w-full" />
      ) : isError ? (
        <p className="text-sm text-destructive">{t('usage.loadFailed')}</p>
      ) : (
        <>
          <dl className="grid grid-cols-2 gap-2 text-sm">
            <div>
              <dt className="text-xs text-muted-foreground">
                {t('usage.size')}
              </dt>
              <dd className="font-medium tabular-nums">
                {sizeLabel(data.bytes, data.at_least, (value) =>
                  t('usage.atLeast', { value }),
                )}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">
                {t('usage.objects')}
              </dt>
              <dd className="font-medium tabular-nums">
                {data.at_least
                  ? t('usage.atLeast', { value: data.objects })
                  : data.objects}
              </dd>
            </div>
          </dl>
          {data.quota_bytes > 0 ? (
            <p className="text-xs text-muted-foreground">
              {t('usage.quota', { size: formatAssetSize(data.quota_bytes) })}
            </p>
          ) : null}
          <form onSubmit={apply} className="space-y-1">
            <Label htmlFor="asset-egress" className="text-xs">
              {t('usage.egress')}
            </Label>
            <div className="flex gap-2">
              <Input
                id="asset-egress"
                type="number"
                min={0}
                step="any"
                inputMode="decimal"
                className="h-7"
                value={draft}
                onChange={(e) => {
                  setDraft(e.target.value)
                }}
              />
              <Button type="submit" size="sm" variant="outline">
                {t('usage.apply')}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              {t('usage.egressHint')}
            </p>
          </form>
          <table className="w-full text-sm">
            <caption className="sr-only">{t('usage.title')}</caption>
            <thead className="text-xs text-muted-foreground">
              <tr>
                <th scope="col" className="text-left font-normal">
                  <span className="sr-only">{t('usage.thisBucket')}</span>
                </th>
                <th scope="col" className="px-2 text-right font-normal">
                  {t('usage.storage')}
                </th>
                <th scope="col" className="px-2 text-right font-normal">
                  {t('usage.egressCost')}
                </th>
                <th scope="col" className="pl-2 text-right font-normal">
                  {t('usage.total')}
                </th>
              </tr>
            </thead>
            <tbody>
              <CostRow
                strong
                label={`${t('usage.thisBucket')} (${presetName(data.cost.current.preset)})`}
                line={data.cost.current}
              />
            </tbody>
            <tbody>
              <tr>
                <th
                  scope="colgroup"
                  colSpan={4}
                  className="pt-2 text-left text-xs font-normal text-muted-foreground"
                >
                  {t('usage.compare')}
                </th>
              </tr>
              {data.cost.compare.map((line) => (
                <CostRow
                  key={line.preset}
                  label={presetName(line.preset)}
                  line={line}
                />
              ))}
            </tbody>
          </table>
          <p className="text-xs text-muted-foreground">
            {t('usage.priceDate', { date: data.cost.price_table_date })}{' '}
            {data.cost.override_envs.length > 0
              ? t('usage.overrides', {
                  envs: data.cost.override_envs.join(', '),
                })
              : null}
          </p>
          {data.cost.notes.map((n) => (
            <p key={n} className="text-xs text-muted-foreground">
              {n}
            </p>
          ))}
        </>
      )}
    </section>
  )
}
