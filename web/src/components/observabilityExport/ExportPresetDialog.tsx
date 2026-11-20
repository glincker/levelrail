import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { SecretInput } from '@/components/ui/secret-input'
import { Switch } from '@/components/ui/switch'
import { InfoTip } from '../kit'
import {
  useCreateObservabilityExport,
  useTestObservabilityExport,
  type ExportPreset,
  type ExportSignalName,
  type ExportTestResult,
  type PresetField,
} from '../../queries/observabilityExports'
import { PresetGlyph } from './presetBrand'
import { TestResults } from './TestResults'

function initialValues(preset: ExportPreset): Record<string, string> {
  return Object.fromEntries(preset.fields.map((f) => [f.key, f.default ?? '']))
}

function initialSignals(preset: ExportPreset): ExportSignalName[] {
  return preset.signals.filter((s) => s.default).map((s) => s.signal)
}

function FieldInput({
  field,
  id,
  value,
  onChange,
}: Readonly<{
  field: PresetField
  id: string
  value: string
  onChange: (v: string) => void
}>) {
  if (field.choices && field.choices.length > 0) {
    return (
      <NativeSelect
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full"
      >
        {field.choices.map((c) => (
          <option key={c.value} value={c.value}>
            {c.label}
          </option>
        ))}
      </NativeSelect>
    )
  }
  if (field.secret) {
    return (
      <SecretInput
        id={id}
        value={value}
        placeholder={field.placeholder}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  }
  return (
    <Input
      id={id}
      value={value}
      placeholder={field.placeholder}
      onChange={(e) => onChange(e.target.value)}
    />
  )
}

export function ExportPresetDialog({
  preset,
  onClose,
}: Readonly<{ preset: ExportPreset; onClose: () => void }>) {
  const { t } = useTranslation('settings')
  const create = useCreateObservabilityExport()
  const test = useTestObservabilityExport()
  const [name, setName] = useState(preset.id)
  const [values, setValues] = useState(() => initialValues(preset))
  const [signals, setSignals] = useState(() => initialSignals(preset))
  const [result, setResult] = useState<ExportTestResult | null>(null)

  const missing = preset.fields.some(
    (f) => f.required && (values[f.key] ?? '').trim() === '',
  )
  const canCreate =
    name.trim() !== '' && !missing && signals.length > 0 && !create.isPending

  function toggle(signal: ExportSignalName, on: boolean) {
    setSignals((cur) =>
      on ? [...cur, signal] : cur.filter((s) => s !== signal),
    )
  }

  function submit() {
    create.mutate(
      { preset: preset.id, name: name.trim(), fields: values, signals },
      {
        onSuccess: (created) =>
          test.mutate(created.id, { onSuccess: setResult }),
      },
    )
  }

  const done = create.isSuccess
  const error = create.error?.message ?? test.error?.message

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <PresetGlyph id={preset.id} />
            {t('obsExport.dialog.title', { name: preset.name })}
          </DialogTitle>
          <DialogDescription>{preset.description}</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault()
            if (canCreate && !done) submit()
          }}
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="obs-export-name">
                {t('obsExport.dialog.name')}
              </Label>
              <Input
                id="obs-export-name"
                value={name}
                disabled={done}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            {preset.fields.map((f) => (
              <div key={f.key} className="space-y-1">
                <div className="flex items-center gap-1">
                  <Label htmlFor={`obs-export-${f.key}`}>
                    {f.label}
                    {f.required ? (
                      <span aria-hidden="true" className="text-tone-danger">
                        *
                      </span>
                    ) : null}
                  </Label>
                  {f.help ? (
                    <InfoTip
                      label={t('obsExport.dialog.help', { field: f.label })}
                    >
                      {f.help}
                    </InfoTip>
                  ) : null}
                </div>
                <FieldInput
                  field={f}
                  id={`obs-export-${f.key}`}
                  value={values[f.key] ?? ''}
                  onChange={(v) => setValues((cur) => ({ ...cur, [f.key]: v }))}
                />
              </div>
            ))}
          </div>
          <div className="flex flex-wrap items-center gap-4">
            {preset.signals.map((s) => (
              <div key={s.signal} className="flex items-center gap-2">
                <Switch
                  id={`obs-export-signal-${s.signal}`}
                  size="sm"
                  checked={signals.includes(s.signal)}
                  disabled={done}
                  onCheckedChange={(on) => toggle(s.signal, on)}
                />
                <Label htmlFor={`obs-export-signal-${s.signal}`}>
                  {t(`obsExport.signal.${s.signal}`)}
                </Label>
              </div>
            ))}
          </div>
          {error ? (
            <p role="alert" className="text-sm text-tone-danger">
              {error}
            </p>
          ) : null}
          {test.isPending ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t('obsExport.test.running')}
            </p>
          ) : null}
          {result ? <TestResults result={result} /> : null}
          <DialogFooter>
            {done ? (
              <>
                {result && !result.ok ? (
                  <Button
                    type="button"
                    variant="outline"
                    disabled={test.isPending}
                    onClick={() =>
                      create.data &&
                      test.mutate(create.data.id, { onSuccess: setResult })
                    }
                  >
                    {t('obsExport.test.retry')}
                  </Button>
                ) : null}
                <Button type="button" onClick={onClose}>
                  {t('obsExport.dialog.done')}
                </Button>
              </>
            ) : (
              <>
                <Button type="button" variant="outline" onClick={onClose}>
                  {t('obsExport.dialog.cancel')}
                </Button>
                <Button type="submit" disabled={!canCreate}>
                  {create.isPending
                    ? t('obsExport.dialog.creating')
                    : t('obsExport.dialog.create')}
                </Button>
              </>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
