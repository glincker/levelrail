import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { MinusGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'

// Bounded integer stepper; min and max come from the caller, never hardcoded here.
export function NumberStepper({
  value,
  onChange,
  min,
  max,
  label,
}: {
  value: number
  onChange: (value: number) => void
  min: number
  max: number
  label: string
}) {
  const { t } = useTranslation('uiShared')
  const id = useId()
  return (
    <div
      role="group"
      aria-labelledby={id}
      className="inline-flex items-center gap-1"
    >
      <span id={id} className="sr-only">
        {label}
      </span>
      <Button
        variant="outline"
        size="icon"
        aria-label={t('stepper.decrease')}
        disabled={value <= min}
        onClick={() => onChange(Math.max(min, value - 1))}
      >
        <MinusGlassIcon aria-hidden="true" />
      </Button>
      <output
        aria-live="polite"
        className="min-w-8 text-center text-sm font-medium tabular-nums"
      >
        {value}
      </output>
      <Button
        variant="outline"
        size="icon"
        aria-label={t('stepper.increase')}
        disabled={value >= max}
        onClick={() => onChange(Math.min(max, value + 1))}
      >
        <PlusGlassIcon aria-hidden="true" />
      </Button>
    </div>
  )
}
