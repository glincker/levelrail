import { MagicWandIcon } from '@phosphor-icons/react/dist/ssr'
import { ActionMenu } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { LB_PRESETS, type LbPreset } from './presets'

export function PresetPicker({
  onPick,
}: {
  onPick: (preset: LbPreset) => void
}) {
  return (
    <ActionMenu
      trigger={
        <Button type="button" variant="outline" size="sm">
          <MagicWandIcon data-icon="inline-start" />
          Presets
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
