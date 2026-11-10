import { useTranslation } from 'react-i18next'
import { selectableCardClass } from './ui/selectable-card'
import { RadioGroup, RadioGroupItem } from './ui/radio-group'
import { cn } from '@/lib/utils'
import { InfoTip } from './kit'
import type { PreviewMode } from '../types/preview'

const MODES: readonly PreviewMode[] = ['off', 'metadata', 'screenshot']

function isMode(v: unknown): v is PreviewMode {
  return typeof v === 'string' && (MODES as readonly string[]).includes(v)
}

export interface PreviewModeSelectorProps {
  value: PreviewMode
  defaultMode: PreviewMode
  disabled?: boolean
  onChange: (mode: PreviewMode) => void
}

export function PreviewModeSelector({
  value,
  defaultMode,
  disabled,
  onChange,
}: Readonly<PreviewModeSelectorProps>) {
  const { t } = useTranslation('previews')
  return (
    <RadioGroup
      aria-label={t('previewMode.groupLabel')}
      value={value}
      disabled={disabled}
      onValueChange={(v) => {
        if (isMode(v)) onChange(v)
      }}
      className="grid gap-2"
    >
      {MODES.map((mode) => {
        const id = `preview-mode-${mode}`
        const selected = value === mode
        const label = t(`previewMode.${mode}.label`)
        return (
          <div
            key={mode}
            className={cn(
              'flex items-start gap-3 rounded-md p-3',
              selectableCardClass(selected),
              disabled && 'opacity-60',
            )}
          >
            <label
              className={cn(
                'grid min-w-0 flex-1 cursor-pointer grid-cols-[auto_1fr] items-start gap-x-3 gap-y-0.5 text-sm font-medium text-foreground',
                disabled && 'cursor-not-allowed',
              )}
            >
              <RadioGroupItem
                value={mode}
                aria-labelledby={`${id}-label`}
                aria-describedby={`${id}-summary`}
                className="row-span-2 mt-0.5"
              />
              <span id={`${id}-label`}>
                {mode === defaultMode
                  ? t('previewMode.default', { label })
                  : label}
              </span>
              <span
                id={`${id}-summary`}
                className="font-normal text-muted-foreground"
              >
                {t(`previewMode.${mode}.summary`)}
              </span>
            </label>
            <InfoTip label={t('previewMode.about', { label })}>
              {t(`previewMode.${mode}.cost`)}
            </InfoTip>
          </div>
        )
      })}
    </RadioGroup>
  )
}
