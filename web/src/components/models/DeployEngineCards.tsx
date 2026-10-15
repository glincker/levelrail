import { BrandTile } from '../BrandLogoBadge'
import { TemplateLogo } from '../TemplateLogo'
import { TEMPLATE_LOGO_LOADERS } from '../../lib/templateLogos'
import { MODEL_ENGINES } from '../../lib/models'
import type { ModelEngine } from '../../types/models'
import { cn } from '@/lib/utils'

// An engine thesvg has no mark for (llama.cpp) falls back to the monogram tile.
function EngineMark({ id, label }: { id: ModelEngine; label: string }) {
  const hasLogo = id in TEMPLATE_LOGO_LOADERS
  return (
    <BrandTile name={id} label={label} size="sm">
      {hasLogo ? (
        <TemplateLogo
          id={id}
          className="size-5"
          fallback={
            <span className="font-mono text-sm font-medium uppercase">
              {label.slice(0, 1)}
            </span>
          }
        />
      ) : undefined}
    </BrandTile>
  )
}

// Radio group rendered as cards: arrow keys move the selection, Tab leaves.
export function DeployEngineCards({
  value,
  onChange,
  legend,
}: {
  value: ModelEngine
  onChange: (engine: ModelEngine) => void
  legend: string
}) {
  return (
    <fieldset className="min-w-0">
      <legend className="mb-1.5 text-sm font-medium text-foreground">
        {legend}
      </legend>
      <div className="grid grid-cols-3 gap-2">
        {MODEL_ENGINES.map((engine) => (
          <label
            key={engine.id}
            className={cn(
              'flex cursor-pointer flex-col items-center gap-2 rounded-lg border border-border bg-card px-2 py-3 text-center text-sm transition-colors hover:bg-muted/50',
              'has-checked:border-primary has-checked:bg-primary/5 has-focus-visible:ring-3 has-focus-visible:ring-ring/50',
            )}
          >
            <input
              type="radio"
              name="model-engine"
              value={engine.id}
              checked={value === engine.id}
              onChange={() => {
                onChange(engine.id)
              }}
              className="sr-only"
            />
            <EngineMark id={engine.id} label={engine.label} />
            <span className="font-medium text-foreground">{engine.label}</span>
          </label>
        ))}
      </div>
    </fieldset>
  )
}
