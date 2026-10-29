import { useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { SquareGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { CliCommand } from '../../lib/cliConsoleProtocol'
import {
  QUICK_ACTIONS,
  QUICK_ACTION_GROUPS,
  buildLine,
  filterQuickActions,
  type QuickAction,
  type QuickParam,
} from '../../lib/cliConsoleQuickActions'

export interface QuickNode {
  id: string
  name: string
}

interface QuickActionsProps {
  commands: CliCommand[]
  appNames: string[]
  nodes: QuickNode[]
  selectedApp: string
  disabled: boolean
  running: boolean
  onRun: (line: string) => void
  onStop: () => void
  className?: string
}

export function SelectedAppRow({
  appName,
  onClear,
}: {
  appName: string
  onClear: () => void
}) {
  const { t } = useTranslation()
  if (appName === '') return null
  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <span>{t('cliConsole.quick.selectedApp')}</span>
      <Badge variant="muted" className="gap-1 font-mono">
        {appName}
        <button
          type="button"
          aria-label={t('cliConsole.quick.clearSelected')}
          className="rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          onClick={onClear}
        >
          <XGlassIcon className="size-3.5" />
        </button>
      </Badge>
    </div>
  )
}

function ParamField({
  param,
  value,
  appNames,
  nodes,
  onChange,
}: {
  param: QuickParam
  value: string
  appNames: string[]
  nodes: QuickNode[]
  onChange: (next: string) => void
}) {
  const { t } = useTranslation()
  const label = t(`cliConsole.quick.params.${param.name}`)
  if (param.kind === 'text') {
    return (
      <label className="flex flex-col gap-1 text-xs">
        {label}
        <Input
          value={value}
          autoComplete="off"
          spellCheck={false}
          className="font-mono"
          onChange={(e) => {
            onChange(e.target.value)
          }}
        />
      </label>
    )
  }
  const options =
    param.kind === 'app'
      ? appNames.map((n) => ({ value: n, label: n }))
      : nodes.map((n) => ({ value: n.id, label: n.name }))
  return (
    <div className="flex flex-col gap-1 text-xs">
      <span>{label}</span>
      <Select
        value={value === '' ? null : value}
        onValueChange={(next) => {
          onChange(next ?? '')
        }}
      >
        <SelectTrigger size="sm" aria-label={label} className="w-full">
          <SelectValue placeholder={t('cliConsole.quick.pick')} />
        </SelectTrigger>
        <SelectContent>
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

interface ActionButtonProps {
  action: QuickAction
  tabIndex: number
  disabled: boolean
  appNames: string[]
  nodes: QuickNode[]
  selectedApp: string
  onFocus: () => void
  onRun: (line: string) => void
  onReview: (action: QuickAction, line: string) => void
}

function ActionButton({
  action,
  tabIndex,
  disabled,
  appNames,
  nodes,
  selectedApp,
  onFocus,
  onRun,
  onReview,
}: ActionButtonProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [values, setValues] = useState<Record<string, string>>({})
  const label = t(`cliConsole.quick.actions.${action.id}.label`)
  const effective = (param: QuickParam): string =>
    values[param.name] ??
    (param.kind === 'app' && appNames.includes(selectedApp) ? selectedApp : '')
  const resolved: Record<string, string> = {}
  for (const p of action.params) resolved[p.name] = effective(p)
  const line = buildLine(action, resolved)
  const common = {
    'data-quick-action': action.id,
    tabIndex,
    disabled,
    onFocus,
  }
  const needsPopover = action.params.length > 0 || action.level === 'change'

  if (!needsPopover) {
    return (
      <Button
        {...common}
        type="button"
        variant="outline"
        size="sm"
        className="justify-start"
        onClick={() => {
          if (line === null) return
          if (action.level === 'destructive') onReview(action, line)
          else onRun(line)
        }}
      >
        {label}
      </Button>
    )
  }

  const confirmLabel =
    action.level === 'destructive'
      ? t('cliConsole.quick.review')
      : t('cliConsole.quick.run')
  return (
    <Popover open={open && !disabled} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            {...common}
            type="button"
            variant="outline"
            size="sm"
            className="justify-start"
          />
        }
      >
        {label}
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64">
        {action.params.map((p) => (
          <ParamField
            key={p.name}
            param={p}
            value={effective(p)}
            appNames={appNames}
            nodes={nodes}
            onChange={(next) => {
              setValues((prev) => ({ ...prev, [p.name]: next }))
            }}
          />
        ))}
        <div className="flex flex-col gap-1 text-xs">
          <span className="text-muted-foreground">
            {t('cliConsole.quick.willRun')}
          </span>
          <code className="rounded-md bg-muted px-2 py-1 font-mono break-all">
            {line ?? t('cliConsole.quick.incomplete')}
          </code>
        </div>
        <Button
          type="button"
          size="sm"
          variant={action.level === 'destructive' ? 'destructive' : 'default'}
          disabled={line === null}
          onClick={() => {
            if (line === null) return
            setOpen(false)
            if (action.level === 'destructive') onReview(action, line)
            else onRun(line)
          }}
        >
          {confirmLabel}
        </Button>
      </PopoverContent>
    </Popover>
  )
}

export function QuickActions({
  commands,
  appNames,
  nodes,
  selectedApp,
  disabled,
  running,
  onRun,
  onStop,
  className,
}: QuickActionsProps) {
  const { t } = useTranslation()
  const toolbarRef = useRef<HTMLDivElement | null>(null)
  const [activeId, setActiveId] = useState<string | null>(null)
  const [review, setReview] = useState<{
    action: QuickAction
    line: string
  } | null>(null)
  const actions = useMemo(
    () => filterQuickActions(QUICK_ACTIONS, commands),
    [commands],
  )
  const firstId = actions[0]?.id ?? null
  const tabbableId =
    activeId !== null && actions.some((a) => a.id === activeId)
      ? activeId
      : firstId

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const keys = [
      'ArrowRight',
      'ArrowDown',
      'ArrowLeft',
      'ArrowUp',
      'Home',
      'End',
    ]
    if (!keys.includes(event.key)) return
    const items = Array.from(
      toolbarRef.current?.querySelectorAll<HTMLButtonElement>(
        'button[data-quick-action]:not(:disabled)',
      ) ?? [],
    )
    if (items.length === 0) return
    const current = items.indexOf(document.activeElement as HTMLButtonElement)
    const forward = event.key === 'ArrowRight' || event.key === 'ArrowDown'
    const step = forward ? 1 : -1
    let next = (current + step + items.length) % items.length
    if (event.key === 'Home') next = 0
    if (event.key === 'End') next = items.length - 1
    event.preventDefault()
    items[next]?.focus()
  }

  return (
    <div className={`flex min-w-0 flex-col gap-2 ${className ?? ''}`}>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium">
          {t('cliConsole.quick.title')}
        </span>
        {running ? (
          <Button
            type="button"
            size="xs"
            variant="destructive"
            onClick={onStop}
          >
            <SquareGlassIcon size="1.2em" />
            {t('cliConsole.quick.stop')}
          </Button>
        ) : null}
      </div>
      {actions.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('cliConsole.quick.connectFirst')}
        </p>
      ) : (
        <div
          ref={toolbarRef}
          role="toolbar"
          aria-label={t('cliConsole.quick.title')}
          onKeyDown={onKeyDown}
          className="flex gap-3 overflow-x-auto pb-1 @md:flex-col @md:overflow-x-visible"
        >
          {QUICK_ACTION_GROUPS.map((group) => {
            const inGroup = actions.filter((a) => a.group === group)
            if (inGroup.length === 0) return null
            const groupLabel = t(`cliConsole.quick.groups.${group}`)
            return (
              <div
                key={group}
                role="group"
                aria-label={groupLabel}
                className="flex shrink-0 items-center gap-1.5 @md:flex-col @md:items-stretch"
              >
                <span className="text-xs text-muted-foreground @md:px-0.5">
                  {groupLabel}
                </span>
                {inGroup.map((action) => (
                  <ActionButton
                    key={action.id}
                    action={action}
                    tabIndex={action.id === tabbableId ? 0 : -1}
                    disabled={disabled || running}
                    appNames={appNames}
                    nodes={nodes}
                    selectedApp={selectedApp}
                    onFocus={() => {
                      setActiveId(action.id)
                    }}
                    onRun={onRun}
                    onReview={(a, line) => {
                      setReview({ action: a, line })
                    }}
                  />
                ))}
              </div>
            )
          })}
        </div>
      )}
      <ConfirmDialog
        open={review !== null}
        onOpenChange={(next) => {
          if (!next) setReview(null)
        }}
        tone="destructive"
        title={t('cliConsole.quick.confirmTitle')}
        description={
          <code className="font-mono break-all">{review?.line ?? ''}</code>
        }
        consequences={
          review?.action.id === 'appRollback'
            ? [t('cliConsole.quick.actions.appRollback.consequence')]
            : review
              ? [t('cliConsole.quick.actions.systemRestart.consequence')]
              : []
        }
        confirmLabel={t('cliConsole.quick.run')}
        onConfirm={() => {
          if (review) onRun(review.line)
          setReview(null)
        }}
      />
    </div>
  )
}
