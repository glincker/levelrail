import { useTranslation } from 'react-i18next'
import {
  BinaryGlassIcon,
  BracesGlassIcon,
  CircleQuestionMarkGlassIcon,
  ClockGlassIcon,
  FingerprintPatternGlassIcon,
  HashGlassIcon,
  SwitchGlassIcon,
  TypeGlassIcon,
} from '@thesvg/ui-react'
import type { ColumnKind } from '../../../lib/explorerData'

const ICONS: Record<ColumnKind, typeof HashGlassIcon> = {
  number: HashGlassIcon,
  text: TypeGlassIcon,
  boolean: SwitchGlassIcon,
  datetime: ClockGlassIcon,
  json: BracesGlassIcon,
  uuid: FingerprintPatternGlassIcon,
  binary: BinaryGlassIcon,
  other: CircleQuestionMarkGlassIcon,
}

export function ColumnTypeIcon({
  kind,
  type,
}: {
  kind: ColumnKind
  type?: string
}) {
  const { t } = useTranslation('databases')
  const Icon = ICONS[kind]
  const label = type ?? t(`viewer.explorer.kind.${kind}`)
  return (
    <span title={label} className="inline-flex shrink-0">
      <Icon
        className="size-3.5 text-muted-foreground"
        aria-label={label}
        role="img"
      />
    </span>
  )
}
