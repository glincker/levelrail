import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'

// Opt-in for deleting everything inside a project, environment or
// organization together with it instead of leaving the members running.
export function CascadeDeleteOption({
  checked,
  onChange,
}: {
  checked: boolean
  onChange: (next: boolean) => void
}) {
  const { t } = useTranslation('common')
  return (
    <div className="flex items-start gap-2">
      <Checkbox
        id="cascade-delete"
        checked={checked}
        onCheckedChange={(v) => onChange(v === true)}
      />
      <Label htmlFor="cascade-delete" className="text-sm leading-snug">
        {t('cascadeDelete.label')}
        <span className="block text-xs text-muted-foreground">
          {t('cascadeDelete.hint')}
        </span>
      </Label>
    </div>
  )
}
