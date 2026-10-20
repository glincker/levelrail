import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from './ui/button'
import { Input } from './ui/input'
import {
  FIELD_OPS,
  formatFieldFilter,
  parseFieldFilter,
  type FieldFilter,
  type FieldOp,
} from '../lib/logFilters'
import type { LogContainer } from '../types/logs'

const SELECT_CLASS =
  'h-8 rounded-md border border-input bg-background px-2 text-xs text-foreground'
const LEVELS = ['', 'debug', 'info', 'warn', 'error'] as const

// Container, stream, minimum level and structured JSON field filters.
export function LogFilterBar({
  containers,
  container,
  stream,
  level,
  fields,
  onContainer,
  onStream,
  onLevel,
  onFields,
}: {
  containers: readonly LogContainer[]
  container?: string
  stream?: 'stdout' | 'stderr'
  level?: string
  fields: readonly FieldFilter[]
  onContainer: (id: string | undefined) => void
  onStream: (s: 'stdout' | 'stderr' | undefined) => void
  onLevel: (l: string | undefined) => void
  onFields: (next: FieldFilter[]) => void
}) {
  const { t } = useTranslation('observability')
  const [key, setKey] = useState('')
  const [op, setOp] = useState<FieldOp>('=')
  const [value, setValue] = useState('')
  const draft = parseFieldFilter(`${key}${op}${value}`)

  const add = () => {
    if (draft) {
      onFields([...fields, draft])
      setKey('')
      setValue('')
    }
  }

  return (
    <div className="mt-3 space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-1 text-xs text-muted-foreground">
          {t('logs.container')}
          <select
            className={SELECT_CLASS}
            value={container ?? ''}
            onChange={(e) => {
              onContainer(e.target.value || undefined)
            }}
          >
            <option value="">{t('logs.allContainers')}</option>
            {containers.map((c) => (
              <option key={c.id} value={c.id}>
                {c.id.slice(0, 12)} ({c.count})
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-1 text-xs text-muted-foreground">
          {t('logs.stream')}
          <select
            className={SELECT_CLASS}
            value={stream ?? ''}
            onChange={(e) => {
              const v = e.target.value
              onStream(v === 'stdout' || v === 'stderr' ? v : undefined)
            }}
          >
            <option value="">{t('logs.anyStream')}</option>
            <option value="stdout">stdout</option>
            <option value="stderr">stderr</option>
          </select>
        </label>
        <label className="flex items-center gap-1 text-xs text-muted-foreground">
          {t('logs.minLevel')}
          <select
            className={SELECT_CLASS}
            value={level ?? ''}
            onChange={(e) => {
              onLevel(e.target.value || undefined)
            }}
          >
            {LEVELS.map((l) => (
              <option key={l} value={l}>
                {l === '' ? t('logs.anyLevel') : l}
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs text-muted-foreground">
          {t('logs.jsonFields')}
        </span>
        <Input
          value={key}
          onChange={(e) => {
            setKey(e.target.value)
          }}
          placeholder={t('logs.fieldKey')}
          aria-label={t('logs.fieldKey')}
          className="h-8 w-32 font-mono text-xs"
        />
        <select
          className={SELECT_CLASS}
          value={op}
          aria-label={t('logs.fieldOp')}
          onChange={(e) => {
            setOp(e.target.value as FieldOp)
          }}
        >
          {FIELD_OPS.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
        <Input
          value={value}
          onChange={(e) => {
            setValue(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              add()
            }
          }}
          placeholder={t('logs.fieldValue')}
          aria-label={t('logs.fieldValue')}
          className="h-8 w-32 font-mono text-xs"
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={!draft}
          onClick={add}
        >
          <PlusIcon className="size-3.5" aria-hidden="true" />
          {t('logs.addFilter')}
        </Button>
      </div>
      {fields.length > 0 ? (
        <ul className="flex flex-wrap gap-1.5">
          {fields.map((f, i) => (
            <li
              key={`${formatFieldFilter(f)}-${i}`}
              className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 font-mono text-xs"
            >
              {formatFieldFilter(f)}
              <button
                type="button"
                aria-label={t('logs.removeFilter', {
                  filter: formatFieldFilter(f),
                })}
                onClick={() => {
                  onFields(fields.filter((_, j) => j !== i))
                }}
                className="text-muted-foreground hover:text-foreground"
              >
                <XIcon className="size-3" aria-hidden="true" />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
