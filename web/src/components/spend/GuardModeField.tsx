import { useTranslation } from 'react-i18next'
import type { SpendGuardMode } from '../../types/costExplorer'

const MODES: SpendGuardMode[] = ['alert', 'block_deploys']

/** GuardModeField picks whether reaching the limit also refuses new deploys. */
export function GuardModeField({
  value,
  onChange,
  name,
}: {
  value: SpendGuardMode
  onChange: (mode: SpendGuardMode) => void
  name: string
}) {
  const { t } = useTranslation('budget')
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t('guard.legend')}</legend>
      <div className="grid gap-2 sm:grid-cols-2">
        {MODES.map((m) => (
          <label
            key={m}
            className="flex cursor-pointer items-start gap-2 rounded-lg border border-border p-2.5 text-sm has-[:checked]:border-ring has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring"
          >
            <input
              type="radio"
              name={name}
              value={m}
              checked={value === m}
              onChange={() => onChange(m)}
              className="mt-0.5"
            />
            <span className="min-w-0 space-y-0.5">
              <span className="block font-medium">{t(`guard.${m}.label`)}</span>
              <span className="block text-xs text-muted-foreground">
                {t(`guard.${m}.help`)}
              </span>
            </span>
          </label>
        ))}
      </div>
    </fieldset>
  )
}
