import { useTranslation } from 'react-i18next'
import { WandGlassIcon } from '@thesvg/ui-react'
import { ActionMenu } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { LB_PRESETS, type LbPreset } from './presets'

export function PresetPicker({
  onPick,
}: {
  onPick: (preset: LbPreset) => void
}) {
  const { t } = useTranslation('statePlaceholders')
  return (
    <ActionMenu
      trigger={
        <Button type="button" variant="outline" size="sm">
          <WandGlassIcon size="1.4em" data-icon="inline-start" />
          {t('lb.configure.presets')}
        </Button>
      }
      items={LB_PRESETS.map((p) => ({
        id: p.id,
        label: p.name,
        description: p.useWhen,
        onSelect: () => onPick(p),
      }))}
    />
  )
}
