import { useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowDownGlassIcon,
  ArrowUpGlassIcon,
  GripVerticalGlassIcon,
  LockGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
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
import {
  buildProxy,
  inferKind,
  parseProxy,
  statusForKind,
  targetState,
  type RuleKind,
  type TargetState,
} from './routesLogic'

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

export interface RowDnd {
  dragging: boolean
  over: boolean
  onStart: () => void
  onOver: () => void
  onDrop: () => void
  onEnd: () => void
}

const KINDS: readonly RuleKind[] = ['redirect', 'rewrite', 'proxy', 'status']

const STATE_DOT: Record<TargetState, string> = {
  ok: 'bg-tone-success',
  missing: 'bg-tone-danger',
  self: 'bg-tone-warning',
}

export function StateDot({ state }: Readonly<{ state?: TargetState }>) {
  const { t } = useTranslation('pagesSites')
  if (!state) return null
  const label = t(`routes.state.${state}`)
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className={`inline-block size-2 shrink-0 rounded-full ${STATE_DOT[state]}`}
    />
  )
}

export function KindChip({ kind }: Readonly<{ kind: RuleKind }>) {
  const { t } = useTranslation('pagesSites')
  const variant =
    kind === 'proxy' ? 'accent' : kind === 'rewrite' ? 'info' : 'muted'
  return <Badge variant={variant}>{t(`routes.kind.${kind}`)}</Badge>
}

function TargetFields({
  row,
  kind,
  apps,
  self,
  describedBy,
  invalid,
  onChange,
  onEnter,
}: Readonly<{
  row: DraftRedirect
  kind: RuleKind
  apps: readonly string[]
  self: string
  describedBy?: string
  invalid: boolean
  onChange: (patch: Partial<DraftRedirect>) => void
  onEnter: (e: KeyboardEvent<HTMLInputElement>) => void
}>) {
  const { t } = useTranslation('pagesSites')
  if (kind !== 'proxy') {
    return (
      <Input
        aria-label={t('routes.to')}
        aria-invalid={invalid}
        aria-describedby={describedBy}
        value={row.to}
        placeholder="/new"
        className="min-w-28 flex-1 basis-32 font-mono"
        onChange={(e) => onChange({ to: e.target.value })}
        onKeyDown={onEnter}
      />
    )
  }
  const p = parseProxy(row.to) ?? { app: '', path: '' }
  const choices = apps.filter((a) => a !== self)
  const listed = p.app === '' || choices.includes(p.app)
  return (
    <>
      <StateDot state={targetState(row.to, self, apps)} />
      <NativeSelect
        aria-label={t('routes.targetApp')}
        aria-invalid={invalid}
        aria-describedby={describedBy}
        value={p.app}
        className="w-36"
        onChange={(e) => onChange({ to: buildProxy(e.target.value, p.path) })}
      >
        <option value="">{t('routes.pickApp')}</option>
        {listed ? null : <option value={p.app}>{p.app}</option>}
        {choices.map((a) => (
          <option key={a} value={a}>
            {a}
          </option>
        ))}
      </NativeSelect>
      <Input
        aria-label={t('routes.targetPath')}
        value={p.path}
        placeholder="/v1/:splat"
        className="min-w-24 flex-1 basis-28 font-mono"
        onChange={(e) => onChange({ to: buildProxy(p.app, e.target.value) })}
        onKeyDown={onEnter}
      />
    </>
  )
}

export function RedirectEditorRow({
  row,
  index,
  count,
  error,
  apps,
  self,
  dnd,
  highlight,
  onChange,
  onMove,
  onRemove,
  onEnterLast,
}: Readonly<{
  row: DraftRedirect
  index: number
  count: number
  error?: string
  apps: readonly string[]
  self: string
  dnd: RowDnd
  highlight: boolean
  onChange: (patch: Partial<DraftRedirect>) => void
  onMove: (to: number) => void
  onRemove: () => void
  onEnterLast: () => void
}>) {
  const { t } = useTranslation('pagesSites')
  const [armed, setArmed] = useState(false)
  const errId = `${row.id}-err`
  const kind = inferKind(row.to, Number(row.status))
  const showStatus = kind === 'redirect' || kind === 'status'
  return (
    <li
      draggable={armed}
      onDragStart={(e) => {
        e.dataTransfer.effectAllowed = 'move'
        dnd.onStart()
      }}
      onDragOver={(e) => {
        e.preventDefault()
        dnd.onOver()
      }}
      onDrop={(e) => {
        e.preventDefault()
        dnd.onDrop()
      }}
      onDragEnd={() => {
        setArmed(false)
        dnd.onEnd()
      }}
      className={cn(
        'space-y-1 px-3 py-2',
        dnd.dragging && 'opacity-50',
        dnd.over && 'bg-muted/50',
        highlight && 'bg-tone-info-soft',
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span
          aria-hidden="true"
          title={t('routes.drag')}
          className="cursor-grab text-muted-foreground"
          onPointerDown={() => setArmed(true)}
          onPointerUp={() => setArmed(false)}
        >
          <GripVerticalGlassIcon className="size-4" />
        </span>
        <NativeSelect
          aria-label={t('routes.kindLabel')}
          value={kind}
          className="w-28"
          onChange={(e) => {
            const next = e.target.value as RuleKind
            const status = statusForKind(next, row.status)
            const to =
              next === 'proxy'
                ? buildProxy('', '')
                : kind === 'proxy'
                  ? ''
                  : row.to
            onChange({ status, to })
          }}
        >
          {KINDS.map((k) => (
            <option key={k} value={k}>
              {t(`routes.kind.${k}`)}
            </option>
          ))}
        </NativeSelect>
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
        <TargetFields
          row={row}
          kind={kind}
          apps={apps}
          self={self}
          invalid={Boolean(error)}
          describedBy={error ? errId : undefined}
          onChange={onChange}
          onEnter={(e) => {
            if (e.key === 'Enter' && index === count - 1) {
              e.preventDefault()
              onEnterLast()
            }
          }}
        />
        {showStatus ? (
          <NativeSelect
            aria-label={t('routes.status')}
            value={row.status}
            className="w-20"
            onChange={(e) => onChange({ status: e.target.value })}
          >
            {REDIRECT_STATUSES.filter((s) =>
              kind === 'status' ? Number(s) >= 400 : Number(s) < 400,
            ).map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </NativeSelect>
        ) : null}
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
  state,
  highlight,
}: Readonly<{
  rule: EffectiveRedirect
  state?: TargetState
  highlight: boolean
}>) {
  return (
    <li
      className={cn(
        'flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-1.5 text-xs',
        highlight && 'bg-tone-info-soft',
      )}
    >
      <KindChip kind={inferKind(rule.to, rule.status)} />
      <code className="font-mono break-all">{rule.from}</code>
      <span aria-hidden="true" className="text-muted-foreground">
        {'→'}
      </span>
      <StateDot state={state} />
      <code className="min-w-0 flex-1 font-mono break-all">{rule.to}</code>
      <Badge variant="muted">{rule.status}</Badge>
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
