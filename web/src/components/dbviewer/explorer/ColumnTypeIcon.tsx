import { useTranslation } from 'react-i18next'
import {
  BracketsCurlyIcon,
  ClockIcon,
  FingerprintIcon,
  HashIcon,
  QuestionIcon,
  TextTIcon,
  ToggleLeftIcon,
  BinaryIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { ColumnKind } from '../../../lib/explorerData'

const ICONS: Record<ColumnKind, typeof HashIcon> = {
  number: HashIcon,
  text: TextTIcon,
  boolean: ToggleLeftIcon,
  datetime: ClockIcon,
  json: BracketsCurlyIcon,
  uuid: FingerprintIcon,
  binary: BinaryIcon,
  other: QuestionIcon,
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
