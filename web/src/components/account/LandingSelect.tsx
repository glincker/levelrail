import { useTranslation } from 'react-i18next'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { LANDING, landingFor } from './preferenceOptions'

/** LandingSelect picks the page Levelrail opens after sign-in. */
export function LandingSelect({
  id,
  value,
  onChange,
}: {
  id: string
  value: string
  onChange: (next: string) => void
}) {
  const { t } = useTranslation('account')
  return (
    <Select
      value={value in LANDING ? value : ''}
      onValueChange={(v) => {
        if (typeof v === 'string') onChange(v)
      }}
    >
      <SelectTrigger id={id} className="h-9 w-full">
        <SelectValue>
          {(v: string) => {
            const item = landingFor(v)
            return (
              <span className="flex items-center gap-2">
                {item.icon}
                {t(item.label)}
              </span>
            )
          }}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {Object.entries(LANDING).map(([path, item]) => (
          <SelectItem key={path || 'default'} value={path}>
            <span className="flex items-center gap-2">
              {item.icon}
              <span className="flex flex-col">
                <span>{t(item.label)}</span>
                <span className="text-xs text-muted-foreground">
                  {t(item.hint)}
                </span>
              </span>
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
