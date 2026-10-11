import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { toLocalInput } from '../lib/observabilityFormat'

export function CustomRangePicker({
  from,
  to,
  onApply,
}: {
  from: Date
  to: Date
  onApply: (from: Date, to: Date) => void
}) {
  const { t } = useTranslation('observability')
  const [start, setStart] = useState(toLocalInput(from))
  const [end, setEnd] = useState(toLocalInput(to))
  const s = new Date(start)
  const e = new Date(end)
  const valid =
    !Number.isNaN(s.getTime()) && !Number.isNaN(e.getTime()) && s < e

  return (
    <form
      className="flex flex-wrap items-center gap-2"
      onSubmit={(ev) => {
        ev.preventDefault()
        if (valid) {
          onApply(s, e)
        }
      }}
    >
      <label className="flex items-center gap-1 text-xs text-muted-foreground">
        {t('range.from')}
        <Input
          type="datetime-local"
          value={start}
          onChange={(ev) => {
            setStart(ev.target.value)
          }}
          className="h-8 w-auto text-xs"
        />
      </label>
      <label className="flex items-center gap-1 text-xs text-muted-foreground">
        {t('range.to')}
        <Input
          type="datetime-local"
          value={end}
          onChange={(ev) => {
            setEnd(ev.target.value)
          }}
          className="h-8 w-auto text-xs"
        />
      </label>
      <Button type="submit" size="sm" variant="outline" disabled={!valid}>
        {t('range.apply')}
      </Button>
    </form>
  )
}
