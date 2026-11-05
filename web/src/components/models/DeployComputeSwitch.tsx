import {
  SelectedMark,
  selectableCardClass,
} from '@/components/ui/selectable-card'
import { CpuGlassIcon, ZapGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import type { ModelCompute } from '../../types/modelsCpu'

// Radio pair: run on a GPU node (the default) or on this server's CPU.
export function DeployComputeSwitch({
  value,
  onChange,
}: {
  value: ModelCompute
  onChange: (next: ModelCompute) => void
}) {
  const { t } = useTranslation('models')
  const options: {
    id: ModelCompute
    label: string
    hint: string
    icon: typeof CpuGlassIcon
  }[] = [
    {
      id: 'gpu',
      label: t('cpu.switch.gpu'),
      hint: t('cpu.switch.gpuHint'),
      icon: ZapGlassIcon,
    },
    {
      id: 'cpu',
      label: t('cpu.switch.cpu'),
      hint: t('cpu.switch.cpuHint'),
      icon: CpuGlassIcon,
    },
  ]
  return (
    <fieldset className="min-w-0">
      <legend className="sr-only">{t('cpu.switch.legend')}</legend>
      <div className="grid gap-2 sm:grid-cols-2">
        {options.map((o) => (
          <label
            key={o.id}
            className={selectableCardClass(
              value === o.id,
              'flex cursor-pointer items-start gap-3 rounded-lg p-3 pr-7 text-sm has-focus-visible:ring-2 has-focus-visible:ring-state-accent',
            )}
          >
            <SelectedMark selected={value === o.id} />
            <input
              type="radio"
              name="model-compute"
              value={o.id}
              checked={value === o.id}
              onChange={() => {
                onChange(o.id)
              }}
              className="sr-only"
            />
            <o.icon className="mt-0.5 size-6 shrink-0" aria-hidden="true" />
            <span>
              <span className="block font-medium text-foreground">
                {o.label}
              </span>
              <span className="block text-xs text-muted-foreground">
                {o.hint}
              </span>
            </span>
          </label>
        ))}
      </div>
    </fieldset>
  )
}
