import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'

/** DeleteDataOption is the opt-in "also destroy the data" switch of a delete dialog. */
export function DeleteDataOption({
  checked,
  onCheckedChange,
  kind,
}: {
  checked: boolean
  onCheckedChange: (next: boolean) => void
  kind: 'database' | 'app'
}) {
  const { t } = useTranslation('backups')
  const id = useId()
  return (
    <div className="flex items-start gap-2 rounded-md border border-border p-3">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(next) => {
          onCheckedChange(next === true)
        }}
        className="mt-0.5"
      />
      <div className="space-y-1">
        <Label htmlFor={id}>{t('deleteData.label')}</Label>
        <p className="text-xs text-muted-foreground">
          {kind === 'database'
            ? t('deleteData.databaseHint')
            : t('deleteData.appHint')}
        </p>
        {checked ? (
          <p className="text-xs text-destructive">{t('deleteData.warning')}</p>
        ) : null}
      </div>
    </div>
  )
}
