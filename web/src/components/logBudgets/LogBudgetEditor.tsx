import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useSetLogBudget } from '../../queries/logBudgets'

/** LogBudgetEditor edits one app's log budget override inline. */
export function LogBudgetEditor({
  app,
  initialMb,
  onDone,
}: {
  app: string
  initialMb: number
  onDone: () => void
}) {
  const { t } = useTranslation('logBudgets')
  const [value, setValue] = useState(String(initialMb))
  const save = useSetLogBudget(app)
  const parsed = Number(value)
  const valid = value.trim() !== '' && Number.isInteger(parsed) && parsed >= 0

  function submit() {
    if (!valid) return
    save.mutate(parsed, {
      onSuccess: () => {
        toast.add({ title: t('saved'), type: 'success' })
        onDone()
      },
      onError: (err) =>
        toast.add({
          title: t('saveFailed'),
          description: err.message,
          type: 'error',
        }),
    })
  }

  return (
    <form
      className="flex items-center gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <Input
        type="number"
        min={0}
        inputMode="numeric"
        className="w-24"
        aria-label={t('editFor', { app })}
        title={t('maxMbHint')}
        value={value}
        onChange={(e) => setValue(e.target.value)}
      />
      <Button type="submit" size="sm" disabled={!valid || save.isPending}>
        {t('save')}
      </Button>
      <Button type="button" size="sm" variant="ghost" onClick={onDone}>
        {t('cancel')}
      </Button>
    </form>
  )
}
