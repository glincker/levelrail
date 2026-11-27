import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useRuntimeSettings,
  useSetRuntimeSetting,
} from '../../queries/runtimeSettings'
import {
  formatNodePrices,
  parseNodePrices,
  type NodePriceRow,
} from '../../lib/advisorSavings'

const NODE_PRICES_KEY = 'advisor.node_prices'

function Editor({ initial }: { initial: string }) {
  const { t } = useTranslation('resourceAdvisor')
  const save = useSetRuntimeSetting()
  const [rows, setRows] = useState<NodePriceRow[]>(() => {
    const parsed = parseNodePrices(initial)
    return parsed.length > 0 ? parsed : [{ node: '', price: '' }]
  })
  const [invalid, setInvalid] = useState(false)

  const edit = (i: number, patch: Partial<NodePriceRow>) => {
    setRows(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  }

  function submit() {
    const text = formatNodePrices(rows)
    setInvalid(text === null)
    if (text === null) return
    save.mutate(
      { key: NODE_PRICES_KEY, value: text },
      {
        onSuccess: () => {
          toast.add({ type: 'success', title: t('nodePrices.saved') })
        },
      },
    )
  }

  return (
    <form
      className="space-y-2"
      aria-label={t('nodePrices.title')}
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      {rows.map((r, i) => (
        <div key={i} className="flex flex-wrap items-center gap-2">
          <Input
            className="w-40"
            aria-label={t('nodePrices.node')}
            placeholder={t('nodePrices.nodePlaceholder')}
            value={r.node}
            onChange={(e) => {
              edit(i, { node: e.target.value })
            }}
          />
          <Input
            className="w-28"
            inputMode="decimal"
            aria-label={t('nodePrices.price')}
            placeholder={t('nodePrices.pricePlaceholder')}
            value={r.price}
            onChange={(e) => {
              edit(i, { price: e.target.value })
            }}
          />
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              setRows(rows.filter((_, j) => j !== i))
            }}
          >
            {t('nodePrices.remove')}
          </Button>
        </div>
      ))}
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            setRows([...rows, { node: '', price: '' }])
          }}
        >
          {t('nodePrices.add')}
        </Button>
        <Button type="submit" size="sm" disabled={save.isPending}>
          {t('nodePrices.save')}
        </Button>
        {invalid ? (
          <p role="alert" className="text-xs text-destructive">
            {t('nodePrices.invalid')}
          </p>
        ) : null}
        {save.isError ? (
          <p role="alert" className="text-xs text-destructive">
            {save.error.message}
          </p>
        ) : null}
      </div>
    </form>
  )
}

/** Edits advisor.node_prices so reclaimable memory turns into a monthly figure. */
export function NodePriceEditor() {
  const { t } = useTranslation('resourceAdvisor')
  const { data } = useRuntimeSettings()
  if (!data) return null
  const setting = data.settings.find((s) => s.key === NODE_PRICES_KEY)
  const initial = typeof setting?.value === 'string' ? setting.value : ''
  return (
    <section className="space-y-2 rounded-lg border border-border p-3">
      <div>
        <h2 className="text-sm font-medium text-foreground">
          {t('nodePrices.title')}
        </h2>
        <p className="text-xs text-muted-foreground">{t('nodePrices.hint')}</p>
      </div>
      <Editor key={initial} initial={initial} />
    </section>
  )
}
