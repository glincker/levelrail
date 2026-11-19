import { useTranslation } from 'react-i18next'
import {
  ArrowDownGlassIcon,
  ArrowUpGlassIcon,
  LockGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type {
  EffectiveHeader,
  EffectiveRedirect,
  RuleSource,
} from '../../queries/pagesSites'
import {
  REDIRECT_STATUSES,
  formatSet,
  type DraftHeader,
  type DraftRedirect,
} from './rulesDraft'

export function SourceBadge({ source }: Readonly<{ source: RuleSource }>) {
  const { t } = useTranslation('pagesSites')
  return (
    <Badge variant={source === 'ui' ? 'info' : 'muted'}>
      {t(`routes.source.${source}`)}
    </Badge>
  )
}

interface MoveProps {
  index: number
  count: number
  onMove: (to: number) => void
  onRemove: () => void
}

function RowActions({ index, count, onMove, onRemove }: Readonly<MoveProps>) {
  const { t } = useTranslation('pagesSites')
  return (
    <div className="ml-auto flex items-center">
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('routes.moveUp')}
        disabled={index === 0}
        onClick={() => onMove(index - 1)}
      >
        <ArrowUpGlassIcon className="size-4" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('routes.moveDown')}
        disabled={index === count - 1}
        onClick={() => onMove(index + 1)}
      >
        <ArrowDownGlassIcon className="size-4" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('routes.remove')}
        onClick={onRemove}
      >
        <TrashGlassIcon className="size-4" aria-hidden="true" />
      </Button>
    </div>
  )
}

function RowError({ id, message }: Readonly<{ id: string; message?: string }>) {
  if (!message) return null
  return (
    <p id={id} role="alert" className="text-xs text-tone-danger">
      {message}
    </p>
  )
}

export function RedirectEditorRow({
  row,
  index,
  count,
  error,
  onChange,
  onMove,
  onRemove,
  onEnterLast,
}: Readonly<
  {
    row: DraftRedirect
    error?: string
    onChange: (patch: Partial<DraftRedirect>) => void
    onEnterLast: () => void
  } & Omit<MoveProps, 'onMove' | 'onRemove'> & {
      onMove: (to: number) => void
      onRemove: () => void
    }
>) {
  const { t } = useTranslation('pagesSites')
  const errId = `${row.id}-err`
  return (
    <li className="space-y-1 px-3 py-2">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          aria-label={t('routes.from')}
          aria-invalid={Boolean(error)}
          aria-describedby={error ? errId : undefined}
          value={row.from}
          placeholder="/old"
          className="min-w-28 flex-1 basis-32 font-mono"
          onChange={(e) => onChange({ from: e.target.value })}
        />
        <span aria-hidden="true" className="text-muted-foreground">
          {'→'}
        </span>
        <Input
          aria-label={t('routes.to')}
          aria-invalid={Boolean(error)}
          value={row.to}
          placeholder="/new"
          className="min-w-28 flex-1 basis-32 font-mono"
          onChange={(e) => onChange({ to: e.target.value })}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && index === count - 1) {
              e.preventDefault()
              onEnterLast()
            }
          }}
        />
        <NativeSelect
          aria-label={t('routes.status')}
          value={row.status}
          className="w-20"
          onChange={(e) => onChange({ status: e.target.value })}
        >
          {REDIRECT_STATUSES.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </NativeSelect>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Switch
            size="sm"
            checked={row.force}
            aria-label={t('routes.force')}
            onCheckedChange={(v) => onChange({ force: v })}
          />
          {t('routes.force')}
        </label>
        <RowActions
          index={index}
          count={count}
          onMove={onMove}
          onRemove={onRemove}
        />
      </div>
      <RowError id={errId} message={error} />
    </li>
  )
}

export function HeaderEditorRow({
  row,
  index,
  count,
  error,
  onChange,
  onMove,
  onRemove,
}: Readonly<
  {
    row: DraftHeader
    error?: string
    onChange: (patch: Partial<DraftHeader>) => void
  } & MoveProps
>) {
  const { t } = useTranslation('pagesSites')
  const errId = `${row.id}-err`
  return (
    <li className="space-y-1 px-3 py-2">
      <div className="flex flex-wrap items-start gap-2">
        <Input
          aria-label={t('routes.headerPath')}
          aria-invalid={Boolean(error)}
          aria-describedby={error ? errId : undefined}
          value={row.path}
          placeholder="/*"
          className="min-w-24 flex-1 basis-28 font-mono"
          onChange={(e) => onChange({ path: e.target.value })}
        />
        <Textarea
          aria-label={t('routes.headerSet')}
          value={row.setText}
          rows={1}
          placeholder={t('routes.headerSetPlaceholder')}
          className="min-h-9 min-w-40 flex-[2] basis-48 py-1.5 font-mono text-xs"
          onChange={(e) => onChange({ setText: e.target.value })}
        />
        <Input
          aria-label={t('routes.headerRemove')}
          value={row.removeText}
          placeholder={t('routes.headerRemovePlaceholder')}
          className="min-w-24 flex-1 basis-28 font-mono"
          onChange={(e) => onChange({ removeText: e.target.value })}
        />
        <RowActions
          index={index}
          count={count}
          onMove={onMove}
          onRemove={onRemove}
        />
      </div>
      <RowError id={errId} message={error} />
    </li>
  )
}

function Locked({
  source,
  origin,
}: Readonly<{ source: RuleSource; origin: string }>) {
  const { t } = useTranslation('pagesSites')
  const hint = t('routes.lockedHint', {
    origin: origin || t(`routes.source.${source}`),
  })
  return (
    <span
      className="flex items-center gap-1.5 text-xs text-muted-foreground"
      title={hint}
    >
      <LockGlassIcon className="size-4" role="img" aria-label={hint} />
      <SourceBadge source={source} />
    </span>
  )
}

export function ReadOnlyRedirect({
  rule,
}: Readonly<{ rule: EffectiveRedirect }>) {
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-1.5 text-xs">
      <code className="font-mono break-all">{rule.from}</code>
      <span aria-hidden="true" className="text-muted-foreground">
        {'→'}
      </span>
      <code className="min-w-0 flex-1 font-mono break-all">{rule.to}</code>
      <Badge variant={rule.status === 200 ? 'info' : 'muted'}>
        {rule.status}
      </Badge>
      {rule.force ? <Badge variant="warning">!</Badge> : null}
      <Locked source={rule.source} origin={rule.origin} />
    </li>
  )
}

export function ReadOnlyHeader({ rule }: Readonly<{ rule: EffectiveHeader }>) {
  const set = formatSet(rule.set)
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-1.5 text-xs">
      <code className="font-mono break-all">{rule.path}</code>
      <code className="min-w-0 flex-1 font-mono break-all whitespace-pre-wrap">
        {[set, ...rule.remove.map((n) => `- ${n}`)].filter(Boolean).join('\n')}
      </code>
      <Locked source={rule.source} origin={rule.origin} />
    </li>
  )
}
