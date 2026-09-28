import {
  CheckCircleIcon,
  ProhibitIcon,
  QuestionIcon,
  WarningIcon,
  XSquareIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { UpstreamView } from './rollup'

export function StateGlyphIcon({ glyph }: { glyph: UpstreamView['glyph'] }) {
  const cls = 'size-3.5'
  switch (glyph) {
    case 'ok':
      return (
        <CheckCircleIcon className={cls} weight="bold" aria-hidden="true" />
      )
    case 'warn':
      return <WarningIcon className={cls} weight="bold" aria-hidden="true" />
    case 'down':
      return <XSquareIcon className={cls} weight="bold" aria-hidden="true" />
    case 'off':
      return <ProhibitIcon className={cls} weight="bold" aria-hidden="true" />
    default:
      return <QuestionIcon className={cls} weight="bold" aria-hidden="true" />
  }
}
