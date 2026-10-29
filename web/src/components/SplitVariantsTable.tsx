import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, TrophyGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  productionShare,
  weightsDirty,
  weightsPayload,
} from '../lib/splitWeights'
import type { SplitTest, SplitVariant } from '../queries/splitTesting'

interface Props {
  test: SplitTest
  busy: boolean
  onSaveWeights: (weights: Record<string, number>) => void
  onPromote: (variant: SplitVariant) => void
}

const STATUS_VARIANT = {
  live: 'success',
  ready: 'info',
  unavailable: 'destructive',
} as const

/** SplitVariantsTable edits variant weights and offers QA links and promote. */
export function SplitVariantsTable({
  test,
  busy,
  onSaveWeights,
  onPromote,
}: Props) {
  const { t } = useTranslation('splitTesting')
  const [edits, setEdits] = useState<Record<string, number>>({})
  const others = test.variants.filter((v) => v.kind !== 'production')
  const prodWeight = productionShare(others.map((v) => edits[v.id] ?? v.weight))
  const payload = weightsPayload(test.variants, edits)
  const dirty = weightsDirty(test.variants, edits)

  async function copy(link: string) {
    try {
      await navigator.clipboard.writeText(link)
      toast.add({ title: t('table.qaCopied'), type: 'success' })
    } catch {
      toast.add({ title: link, type: 'info' })
    }
  }

  return (
    <div className="space-y-3">
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <caption className="sr-only">{t('table.caption')}</caption>
          <thead>
            <tr className="border-b border-border text-left text-xs text-muted-foreground">
              <th scope="col" className="py-2 pr-3 font-medium">
                {t('table.variant')}
              </th>
              <th scope="col" className="py-2 pr-3 font-medium">
                {t('table.status')}
              </th>
              <th scope="col" className="py-2 pr-3 font-medium">
                {t('table.weight')}
              </th>
              <th scope="col" className="py-2 pr-3 text-right font-medium">
                {t('table.requests')}
              </th>
              <th scope="col" className="py-2 font-medium">
                {t('table.actions')}
              </th>
            </tr>
          </thead>
          <tbody>
            {test.variants.map((v) => {
              const isProd = v.kind === 'production'
              const weight = isProd ? prodWeight : (edits[v.id] ?? v.weight)
              return (
                <tr
                  key={v.id}
                  className="border-b border-border/60 align-middle"
                >
                  <td className="py-2 pr-3">
                    <div className="font-medium text-foreground">{v.label}</div>
                    <div className="text-xs text-muted-foreground">
                      {t(`kind.${v.kind}`)}
                      {v.ref ? `: ${v.ref}` : ''}
                    </div>
                  </td>
                  <td className="py-2 pr-3">
                    <Badge variant={STATUS_VARIANT[v.status]}>
                      {t(`status.${v.status}`)}
                    </Badge>
                    {v.status === 'unavailable' ? (
                      <div className="mt-1 text-xs text-muted-foreground">
                        {t('table.unavailableHelp')}
                      </div>
                    ) : null}
                  </td>
                  <td className="py-2 pr-3">
                    <div className="flex items-center gap-2">
                      <input
                        type="range"
                        min={0}
                        max={100}
                        step={1}
                        value={Math.max(0, weight)}
                        disabled={isProd || busy}
                        aria-label={t('table.weightFor', { label: v.label })}
                        onChange={(e) =>
                          setEdits({ ...edits, [v.id]: Number(e.target.value) })
                        }
                        className="w-28 accent-primary"
                      />
                      <span className="w-10 tabular-nums text-foreground">
                        {weight}%
                      </span>
                    </div>
                  </td>
                  <td className="py-2 pr-3 text-right tabular-nums">
                    {v.requests}
                    <span className="ml-1 text-xs text-muted-foreground">
                      ({Math.round(v.share * 100)}%)
                    </span>
                  </td>
                  <td className="py-2">
                    <div className="flex flex-wrap gap-2">
                      {v.qa_link ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          onClick={() => void copy(v.qa_link ?? '')}
                        >
                          <CopyGlassIcon
                            className="size-5"
                            aria-hidden="true"
                          />
                          {t('table.copyQa')}
                        </Button>
                      ) : null}
                      {!isProd ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          disabled={busy || v.status === 'unavailable'}
                          onClick={() => onPromote(v)}
                        >
                          <TrophyGlassIcon
                            className="size-5"
                            aria-hidden="true"
                          />
                          {t('table.promote')}
                        </Button>
                      ) : null}
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Button
          type="button"
          size="sm"
          disabled={busy || !dirty || payload === null}
          onClick={() => {
            if (payload) {
              onSaveWeights(payload)
              setEdits({})
            }
          }}
        >
          {t('table.saveWeights')}
        </Button>
        <span className="text-xs text-muted-foreground" role="status">
          {payload === null
            ? t('table.overshoot')
            : t('table.productionKeeps', { percent: prodWeight })}
        </span>
      </div>
      <p className="text-xs text-muted-foreground">{t('table.qaHelp')}</p>
    </div>
  )
}
