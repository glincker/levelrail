import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

/** AppPicker chooses which app a purge or history view applies to. */
export function AppPicker({
  apps,
  value,
  onChange,
}: Readonly<{
  apps: string[]
  value: string
  onChange: (app: string) => void
}>) {
  const { t } = useTranslation('cdn')
  const id = useId()
  return (
    <div className="max-w-xs space-y-1.5">
      <Label htmlFor={id}>{t('picker.label')}</Label>
      <Select value={value} onValueChange={(v) => v && onChange(String(v))}>
        <SelectTrigger id={id}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {apps.map((a) => (
            <SelectItem key={a} value={a}>
              {a}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
