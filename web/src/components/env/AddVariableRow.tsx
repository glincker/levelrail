import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  LightbulbGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import {
  looksSensitive,
  newRowId,
  type EnvRow,
  type EnvVarType,
} from '../../lib/envRows'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

const KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/
const TYPES: EnvVarType[] = ['plain', 'secret', 'vault']

export interface AddVariableRowProps {
  existingKeys: Set<string>
  secretsConfigured: boolean
  initialKey?: string
  onAdd: (row: EnvRow) => void
  onCancel: () => void
}

export function AddVariableRow({
  existingKeys,
  secretsConfigured,
  initialKey,
  onAdd,
  onCancel,
}: AddVariableRowProps) {
  const { t } = useTranslation('envEditor')
  const [key, setKey] = useState(initialKey ?? '')
  const [type, setType] = useState<EnvVarType>(() =>
    initialKey && secretsConfigured && looksSensitive(initialKey)
      ? 'secret'
      : 'plain',
  )
  const [touchedType, setTouchedType] = useState(false)
  const [value, setValue] = useState('')
  const [path, setPath] = useState('')
  const [field, setField] = useState('')

  const trimmed = key.trim()
  const keyError =
    trimmed === ''
      ? null
      : !KEY_PATTERN.test(trimmed)
        ? t('add.invalidKey')
        : existingKeys.has(trimmed)
          ? t('add.duplicate', { key: trimmed })
          : null
  const suggestSecret =
    type === 'plain' && looksSensitive(trimmed) && secretsConfigured
  const ready =
    trimmed !== '' &&
    keyError === null &&
    (type === 'vault'
      ? path.trim() !== '' && field.trim() !== ''
      : type === 'plain' || value !== '')

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!ready) return
    onAdd({
      id: newRowId(),
      key: trimmed,
      type,
      value: type === 'vault' ? '' : value,
      vault:
        type === 'vault' ? { path: path.trim(), key: field.trim() } : undefined,
      sensitive: type !== 'plain' || looksSensitive(trimmed),
      origin: 'own',
    })
  }

  return (
    <form
      onSubmit={submit}
      className="space-y-2 border-b border-border bg-muted/40 px-3 py-3"
    >
      <div className="grid gap-2 sm:grid-cols-[minmax(0,13rem)_minmax(0,1fr)_auto] sm:items-start">
        <Input
          aria-label={t('add.keyLabel')}
          placeholder="API_KEY"
          className="font-mono"
          autoFocus
          spellCheck={false}
          value={key}
          aria-invalid={keyError !== null}
          onChange={(e) => {
            const next = e.target.value
            setKey(next)
            if (!touchedType && secretsConfigured) {
              setType(looksSensitive(next) ? 'secret' : 'plain')
            }
          }}
        />
        {type === 'vault' ? (
          <div className="grid grid-cols-[minmax(0,2fr)_minmax(0,1fr)] gap-2">
            <Input
              aria-label={t('value.vaultPath')}
              placeholder="secret/data/app"
              className="font-mono"
              value={path}
              onChange={(e) => {
                setPath(e.target.value)
              }}
            />
            <Input
              aria-label={t('value.vaultKey')}
              placeholder="api_key"
              className="font-mono"
              value={field}
              onChange={(e) => {
                setField(e.target.value)
              }}
            />
          </div>
        ) : (
          <Input
            aria-label={t('add.valueLabel')}
            type={type === 'secret' ? 'password' : 'text'}
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
            placeholder={t('value.valuePlaceholder')}
            value={value}
            onChange={(e) => {
              setValue(e.target.value)
            }}
          />
        )}
        <div
          role="radiogroup"
          aria-label={t('add.typeLabel')}
          className="inline-flex rounded-md border border-border p-0.5"
        >
          {TYPES.map((option) => {
            const disabled = option === 'secret' && !secretsConfigured
            return (
              <Button
                key={option}
                type="button"
                role="radio"
                aria-checked={type === option}
                variant={type === option ? 'secondary' : 'ghost'}
                size="sm"
                disabled={disabled}
                title={disabled ? t('add.secretsOff') : undefined}
                onClick={() => {
                  setTouchedType(true)
                  setType(option)
                }}
              >
                {t(`types.${option}`)}
              </Button>
            )
          })}
        </div>
      </div>
      {keyError ? <p className="text-xs text-destructive">{keyError}</p> : null}
      {suggestSecret ? (
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <LightbulbGlassIcon aria-hidden className="size-4" />
          {t('add.suggestSecret')}
        </p>
      ) : null}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
          <XGlassIcon size="1.2em" />
          {t('add.cancel')}
        </Button>
        <Button type="submit" size="sm" disabled={!ready}>
          <CheckGlassIcon size="1.2em" />
          {t('add.add')}
        </Button>
      </div>
    </form>
  )
}
