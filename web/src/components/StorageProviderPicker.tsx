import { BrandTile } from './BrandLogoBadge'
import { STORAGE_PRESET_LABEL } from '../lib/storageProviders'
import { storagePresetBrand } from '../lib/providerBrands'
import type { StorageProvider, StoragePreset } from '../types/storage'

// Radio-style grid of provider presets, each with its brand mark.
export function StorageProviderPicker({
  providers,
  value,
  onChange,
}: {
  providers: StorageProvider[]
  value: StoragePreset
  onChange: (next: StoragePreset) => void
}) {
  return (
    <div
      role="radiogroup"
      aria-label="Storage provider"
      className="grid grid-cols-2 gap-2 sm:grid-cols-3"
    >
      {providers.map((p) => {
        const selected = p.id === value
        return (
          <button
            key={p.id}
            type="button"
            role="radio"
            aria-checked={selected}
            onClick={() => {
              onChange(p.id)
            }}
            className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-left text-sm transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none ${
              selected
                ? 'border-primary bg-primary/5 text-foreground'
                : 'border-border text-muted-foreground hover:bg-muted'
            }`}
          >
            <BrandTile
              name={storagePresetBrand(p.id)}
              label={STORAGE_PRESET_LABEL[p.id] || p.label}
              size="sm"
            />
            <span className="font-medium">
              {STORAGE_PRESET_LABEL[p.id] || p.label}
            </span>
          </button>
        )
      })}
    </div>
  )
}
