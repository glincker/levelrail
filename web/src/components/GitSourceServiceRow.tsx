import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  BUILD_PACKS,
  isBuildTypeValue,
  type AdditionalServiceRow,
} from '../lib/gitSourceForm'

type RowValue = AdditionalServiceRow & { port?: string }

// One editable fan-out row. Additional-service rows have no port; the
// services map passes `withPort` and the error wiring for its own row.
export function GitSourceServiceRow({
  row,
  namePlaceholder,
  pathPlaceholder,
  withPort,
  disabled,
  invalid,
  describedBy,
  removeLabel,
  onChange,
  onRemove,
}: {
  row: RowValue
  namePlaceholder: string
  pathPlaceholder: string
  withPort: boolean
  disabled: boolean
  invalid?: boolean
  describedBy?: string
  removeLabel: string
  onChange: (patch: Partial<RowValue>) => void
  onRemove: () => void
}) {
  return (
    <div className="flex items-start gap-2">
      <Input
        className="font-mono"
        placeholder={namePlaceholder}
        autoComplete="off"
        spellCheck={false}
        value={row.serviceName}
        onChange={(e) => {
          onChange({ serviceName: e.target.value })
        }}
        disabled={disabled}
        aria-label="Service name" // i18n-ignore
        aria-invalid={invalid}
        aria-describedby={describedBy}
      />
      <Select
        value={row.buildType}
        onValueChange={(v) => {
          if (isBuildTypeValue(v)) onChange({ buildType: v })
        }}
      >
        <SelectTrigger
          className="w-40 shrink-0"
          aria-label="Build type" // i18n-ignore
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {BUILD_PACKS.map((pack) => (
            <SelectItem key={pack.value} value={pack.value}>
              {pack.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        className="font-mono"
        placeholder={pathPlaceholder}
        autoComplete="off"
        spellCheck={false}
        value={row.buildPath}
        onChange={(e) => {
          onChange({ buildPath: e.target.value })
        }}
        disabled={disabled || row.buildType === 'railpack'}
        aria-label="Build path" // i18n-ignore
      />
      {withPort ? (
        <Input
          className="w-24 shrink-0 font-mono"
          type="number"
          placeholder="3000"
          value={row.port ?? ''}
          onChange={(e) => {
            onChange({ port: e.target.value })
          }}
          disabled={disabled}
          aria-label="Port" // i18n-ignore
        />
      ) : null}
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={disabled}
        onClick={onRemove}
        aria-label={removeLabel}
      >
        <TrashGlassIcon size="1.4em" />
      </Button>
    </div>
  )
}
