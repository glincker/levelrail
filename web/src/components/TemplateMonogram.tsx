import { cn } from '@/lib/utils'
import { MONOGRAM_TONE_CLASS, monogramTone } from '../lib/templateTint'

/** TemplateMonogram is the fallback mark: first letter on a tint derived from the template id. */
export function TemplateMonogram({ id, name }: { id: string; name: string }) {
  return (
    <span
      data-tone={monogramTone(id)}
      className={cn(
        'flex size-full items-center justify-center rounded-[inherit] text-base font-semibold uppercase',
        MONOGRAM_TONE_CLASS[monogramTone(id)],
      )}
    >
      {name.trim().slice(0, 1)}
    </span>
  )
}
