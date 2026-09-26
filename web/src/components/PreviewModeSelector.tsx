import { cn } from '@/lib/utils'
import { InfoTip } from './kit'
import type { PreviewMode } from '../types/preview'

interface ModeOption {
  mode: PreviewMode
  label: string
  summary: string
  cost: string
}

const OPTIONS: ModeOption[] = [
  {
    mode: 'off',
    label: 'Off',
    summary: 'No preview is made.',
    cost: 'Nothing runs after a deploy and nothing is stored.',
  },
  {
    mode: 'metadata',
    label: 'Metadata',
    summary: 'No browser. Costs about nothing.',
    cost: 'Makes one small request to the app after each deploy and reads its title and social image (og:image). No browser, no container and no image download. Without a social image the preview is a text card. Pages that need a login are skipped.',
  },
  {
    mode: 'screenshot',
    label: 'Screenshot',
    summary: 'Runs a browser for about 10 s per deploy.',
    cost: 'Runs a browser container for about 10 seconds after each deploy and shows the real page. The browser image is about 143 MB, downloaded once and removed again when it sits unused.',
  },
]

export interface PreviewModeSelectorProps {
  value: PreviewMode
  defaultMode: PreviewMode
  disabled?: boolean
  onChange: (mode: PreviewMode) => void
}

export function PreviewModeSelector({
  value,
  defaultMode,
  disabled,
  onChange,
}: PreviewModeSelectorProps) {
  return (
    <div
      role="radiogroup"
      aria-label="Deploy preview mode"
      className="grid gap-2"
    >
      {OPTIONS.map((o) => {
        const id = `preview-mode-${o.mode}`
        const selected = value === o.mode
        return (
          <div
            key={o.mode}
            className={cn(
              'flex items-start gap-2 rounded-md border p-3',
              selected ? 'border-primary bg-muted/50' : 'border-border',
              disabled && 'opacity-60',
            )}
          >
            <label
              htmlFor={id}
              className={cn(
                'flex min-w-0 flex-1 cursor-pointer items-start gap-3',
                disabled && 'cursor-not-allowed',
              )}
            >
              <input
                id={id}
                type="radio"
                name="preview-mode"
                className="mt-1 accent-primary"
                checked={selected}
                disabled={disabled}
                onChange={() => onChange(o.mode)}
              />
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium text-foreground">
                  {o.label}
                  {o.mode === defaultMode ? (
                    <span className="ml-1.5 text-xs font-normal text-muted-foreground">
                      (default)
                    </span>
                  ) : null}
                </span>
                <span className="block text-sm text-muted-foreground">
                  {o.summary}
                </span>
              </span>
            </label>
            <InfoTip label={`About ${o.label} previews`}>{o.cost}</InfoTip>
          </div>
        )
      })}
    </div>
  )
}
