import { ChartLineGlassIcon } from '@thesvg/ui-react'
import { BrandIcon, isBrandIconName } from '../BrandIcon'

// Preset ids that do not match a brand mark name one to one.
const PRESET_LOGOS: Record<string, string> = {
  'grafana-cloud': 'grafana',
  'grafana-loki': 'grafana',
  'otlp-http': 'opentelemetry',
}

export function PresetGlyph({ id }: Readonly<{ id: string }>) {
  const name = PRESET_LOGOS[id] ?? id
  if (isBrandIconName(name)) {
    return <BrandIcon name={name} className="size-6 shrink-0" />
  }
  return <ChartLineGlassIcon className="size-6 shrink-0" aria-hidden="true" />
}
