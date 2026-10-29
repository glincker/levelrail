import {
  BanGlassIcon,
  CircleCheckGlassIcon,
  CircleQuestionMarkGlassIcon,
  SquareXGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import type { UpstreamView } from './rollup'

export function StateGlyphIcon({ glyph }: { glyph: UpstreamView['glyph'] }) {
  const cls = 'size-3.5'
  switch (glyph) {
    case 'ok':
      return (
        <CircleCheckGlassIcon size="1.2em" className={cls} aria-hidden="true" />
      )
    case 'warn':
      return (
        <TriangleAlertGlassIcon
          size="1.2em"
          className={cls}
          aria-hidden="true"
        />
      )
    case 'down':
      return (
        <SquareXGlassIcon size="1.2em" className={cls} aria-hidden="true" />
      )
    case 'off':
      return <BanGlassIcon size="1.2em" className={cls} aria-hidden="true" />
    default:
      return (
        <CircleQuestionMarkGlassIcon
          size="1.2em"
          className={cls}
          aria-hidden="true"
        />
      )
  }
}
