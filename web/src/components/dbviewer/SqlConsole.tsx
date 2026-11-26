import { useEffect, useReducer, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DownloadGlassIcon,
  PlayGlassIcon,
  PlusGlassIcon,
  SaveGlassIcon,
  TriangleAlertGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
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
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { useWriteUnlock } from '../../hooks/useWriteUnlock'
import { confirmName, detectDestructive } from '../../lib/destructiveSql'
import { downloadText, toCsv, toJson } from '../../lib/sqlConsole/export'
import { formatSql } from '../../lib/sqlConsole/format'
import {
  activeTabOf,
  isDirty,
  loadTabs,
  saveTabs,
  tabNumber,
  tabsReducer,
  type ConsoleTab,
  type TabsState,
} from '../../lib/sqlConsole/tabs'
import {
  useDatabaseFullSchema,
  useRunDatabaseQuery,
  useSaveQuery,
} from '../../queries/databaseViewer'
import type { DbQueryResponse } from '../../types/databaseViewer'
import { ConsoleSidebar } from './ConsoleSidebar'
import { ExplainTree } from './ExplainTree'
import { ResultGrid } from './ResultGrid'
import { SqlEditor } from './SqlEditor'
import { WriteUnlockDialog } from './WriteUnlockDialog'

interface Outcome {
  result: DbQueryResponse | null
  error: string | null
}

function initTabs(arg: { database: string; initialSql: string }): TabsState {
  const restored = loadTabs(arg.database)
  return arg.initialSql === ''
    ? restored
    : tabsReducer(restored, { type: 'open', sql: arg.initialSql })
}

// SQL console. Read-only unless an admin explicitly enables writes for
// this page session; the server enforces both (read-only transaction,
// statement guard, admin-only route), this UI only mirrors it.
export function SqlConsole({
  databaseName,
  initialSql = '',
}: {
  databaseName: string
  initialSql?: string
}) {
  const { t } = useTranslation('databases')
  const [state, dispatch] = useReducer(
    tabsReducer,
    { database: databaseName, initialSql },
    initTabs,
  )
  const activeTab: ConsoleTab = activeTabOf(state)
  const sql = activeTab.sql
  const [outcomes, setOutcomes] = useState<Record<string, Outcome>>({})
  const outcome = outcomes[activeTab.id]
  const [schemaWanted, setSchemaWanted] = useState(false)
  const schema = useDatabaseFullSchema(databaseName, schemaWanted)
  const unlock = useWriteUnlock(databaseName)
  const writes = unlock.unlocked
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [destructiveOpen, setDestructiveOpen] = useState(false)
  const [typedObject, setTypedObject] = useState('')
  const [saveOpen, setSaveOpen] = useState(false)
  const [saveName, setSaveName] = useState('')
  const run = useRunDatabaseQuery()
  const save = useSaveQuery(databaseName)

  useEffect(() => {
    saveTabs(databaseName, state)
  }, [databaseName, state])

  const canRun = sql.trim() !== '' && !run.isPending

  function execute(mode: 'read' | 'write' | 'explain', analyze = false) {
    const tabId = activeTab.id
    run.mutate(
      {
        name: databaseName,
        sql,
        mode: mode === 'read' && writes ? 'write' : mode,
        analyze,
        confirm: writes ? databaseName : undefined,
      },
      {
        onSuccess: (data) => {
          setOutcomes((o) => ({ ...o, [tabId]: { result: data, error: null } }))
        },
        onError: (err) => {
          setOutcomes((o) => ({
            ...o,
            [tabId]: { result: null, error: err.message },
          }))
        },
      },
    )
  }

  const destructive = writes ? detectDestructive(sql) : null

  function runClicked() {
    if (destructive) {
      setTypedObject('')
      setDestructiveOpen(true)
      return
    }
    execute('read')
  }

  function onToggleWrites(next: boolean) {
    if (next) {
      setConfirmOpen(true)
      return
    }
    unlock.lock()
  }

  function newTab() {
    dispatch({ type: 'open' })
  }

  function openSave() {
    setSaveName(activeTab.title ?? '')
    setSaveOpen(true)
  }

  // Browsers keep Cmd/Ctrl+T for themselves, so new tab also has a button.
  function onShortcut(e: React.KeyboardEvent<HTMLDivElement>) {
    if (!(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey) {
      return
    }
    const key = e.key.toLowerCase()
    if (key === 'enter' && canRun) {
      e.preventDefault()
      runClicked()
    } else if (key === 's' && sql.trim() !== '') {
      e.preventDefault()
      openSave()
    } else if (key === 't') {
      e.preventDefault()
      newTab()
    }
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_18rem]">
      <div
        role="group"
        aria-label={t('viewer.console.title')}
        className="min-w-0 space-y-3"
        onKeyDown={onShortcut}
      >
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
          <p className="text-xs text-muted-foreground">
            {writes
              ? t('viewer.console.shortcutsHint')
              : `${t('viewer.console.writeToggleHint')} ${t('viewer.console.shortcutsHint')}`}
          </p>
          <label className="flex items-center gap-2 text-sm">
            <Switch checked={writes} onCheckedChange={onToggleWrites} />
            <span>{t('viewer.console.writeToggle')}</span>
          </label>
        </div>

        <div className="flex items-center gap-1 overflow-x-auto border-b border-border">
          <div
            role="tablist"
            aria-label={t('viewer.console.tabs.listLabel')}
            className="flex items-center gap-1"
          >
            {state.tabs.map((tab) => {
              const title =
                tab.title ??
                t('viewer.console.tabs.untitled', {
                  n: tabNumber(state, tab.id),
                })
              const selected = tab.id === state.activeId
              return (
                <div
                  key={tab.id}
                  className={cn(
                    'flex shrink-0 items-center rounded-t-md border border-b-0 text-sm',
                    selected
                      ? 'border-border bg-card'
                      : 'border-transparent text-muted-foreground hover:bg-muted',
                  )}
                >
                  <button
                    type="button"
                    role="tab"
                    aria-selected={selected}
                    className="max-w-44 truncate py-1 pl-3 pr-1 outline-none focus-visible:ring-2 focus-visible:ring-(--glinr-accent-solid)"
                    onClick={() => {
                      dispatch({ type: 'select', id: tab.id })
                    }}
                  >
                    {title}
                    {isDirty(tab) ? (
                      <span
                        className="ml-1.5 inline-block size-1.5 rounded-full bg-primary align-middle"
                        title={t('viewer.console.tabs.unsaved')}
                        role="img"
                        aria-label={t('viewer.console.tabs.unsaved')}
                      />
                    ) : null}
                  </button>
                  <button
                    type="button"
                    className="mr-1 flex size-5 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
                    aria-label={t('viewer.console.tabs.close', { title })}
                    onClick={() => {
                      dispatch({ type: 'close', id: tab.id })
                    }}
                  >
                    <XGlassIcon className="size-3.5" aria-hidden="true" />
                  </button>
                </div>
              )
            })}
          </div>
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            title={t('viewer.console.tabs.new')}
            aria-label={t('viewer.console.tabs.new')}
            onClick={newTab}
          >
            <PlusGlassIcon className="size-5" aria-hidden="true" />
          </Button>
        </div>

        <SqlEditor
          key={activeTab.id}
          value={sql}
          schemas={schema.data?.schemas ?? []}
          onFirstFocus={() => {
            setSchemaWanted(true)
          }}
          onChange={(next) => {
            dispatch({ type: 'edit', id: activeTab.id, sql: next })
          }}
        />
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            size="sm"
            disabled={!canRun}
            variant={writes ? 'destructive' : 'default'}
            onClick={runClicked}
          >
            <PlayGlassIcon className="size-5" aria-hidden="true" />
            {run.isPending
              ? t('viewer.console.running')
              : writes
                ? t('viewer.console.runWrite')
                : t('viewer.console.run')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!canRun || writes}
            onClick={() => {
              execute('explain')
            }}
          >
            {t('viewer.console.explain')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!canRun || writes}
            title={t('viewer.console.explainHint')}
            onClick={() => {
              execute('explain', true)
            }}
          >
            {t('viewer.console.explainAnalyze')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={sql.trim() === ''}
            title={t('viewer.console.editor.formatHint')}
            onClick={() => {
              dispatch({ type: 'edit', id: activeTab.id, sql: formatSql(sql) })
            }}
          >
            {t('viewer.console.editor.format')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={sql.trim() === ''}
            onClick={openSave}
          >
            <SaveGlassIcon className="size-5" aria-hidden="true" />
            {t('viewer.console.saved.save')}
          </Button>
        </div>

        <ResultsView
          databaseName={databaseName}
          result={outcome?.result ?? null}
          error={outcome?.error ?? null}
          pending={run.isPending}
        />
      </div>

      <ConsoleSidebar
        databaseName={databaseName}
        onOpen={(openSql, title) => {
          dispatch({ type: 'open', sql: openSql, title })
        }}
      />

      <WriteUnlockDialog
        databaseName={databaseName}
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        onUnlock={(minutes) => {
          unlock.unlock(minutes)
        }}
      />

      <Dialog open={destructiveOpen} onOpenChange={setDestructiveOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-1.5 text-destructive">
              <TriangleAlertGlassIcon
                className="size-6 text-tone-warning"
                aria-hidden="true"
              />
              {destructive
                ? t('viewer.console.destructive.title', {
                    kind: t(
                      `viewer.console.destructive.kind.${destructive.kind}`,
                    ),
                  })
                : null}
            </DialogTitle>
            <DialogDescription>
              {destructive
                ? t(`viewer.console.destructive.body.${destructive.kind}`, {
                    object:
                      destructive.object ||
                      t('viewer.console.destructive.unknownObject'),
                  })
                : null}
            </DialogDescription>
          </DialogHeader>
          {destructive?.typeToConfirm ? (
            <label className="space-y-1 text-sm">
              <span>
                {t('viewer.console.destructive.typeLabel', {
                  name: confirmName(destructive),
                })}
              </span>
              <Input
                value={typedObject}
                autoComplete="off"
                spellCheck={false}
                onChange={(e) => {
                  setTypedObject(e.target.value)
                }}
              />
            </label>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setDestructiveOpen(false)
              }}
            >
              {t('viewer.console.cancel')}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={
                destructive?.typeToConfirm === true &&
                typedObject !== confirmName(destructive)
              }
              onClick={() => {
                setDestructiveOpen(false)
                execute('read')
              }}
            >
              {t('viewer.console.destructive.run')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={saveOpen} onOpenChange={setSaveOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('viewer.console.saved.dialogTitle')}</DialogTitle>
          </DialogHeader>
          <label className="space-y-1 text-sm">
            <span>{t('viewer.console.saved.nameLabel')}</span>
            <Input
              value={saveName}
              placeholder={t('viewer.console.saved.namePlaceholder')}
              onChange={(e) => {
                setSaveName(e.target.value)
              }}
            />
          </label>
          {save.isError ? (
            <p className="text-sm text-destructive" role="alert">
              {save.error.message}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              disabled={saveName.trim() === '' || save.isPending}
              onClick={() => {
                save.mutate(
                  { name: saveName.trim(), sql },
                  {
                    onSuccess: () => {
                      dispatch({
                        type: 'saved',
                        id: activeTab.id,
                        title: saveName.trim(),
                        sql,
                      })
                      setSaveOpen(false)
                      toast.add({
                        title: t('viewer.console.saved.toast', {
                          name: saveName.trim(),
                        }),
                        type: 'success',
                      })
                    },
                  },
                )
              }}
            >
              {t('viewer.console.saved.submit')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
function ResultsView({
  databaseName,
  result,
  error,
  pending,
}: {
  databaseName: string
  result: DbQueryResponse | null
  error: string | null
  pending: boolean
}) {
  const { t } = useTranslation('databases')
  if (error) {
    return (
      <pre
        className="whitespace-pre-wrap break-words rounded-md border border-destructive/30 bg-destructive/10 p-3 font-mono text-xs text-destructive"
        role="alert"
      >
        {error}
      </pre>
    )
  }
  if (!result) {
    return (
      <p className="rounded-md border border-dashed border-border p-4 text-sm text-muted-foreground">
        {pending
          ? t('viewer.console.running')
          : t('viewer.console.noResultsYet')}
      </p>
    )
  }
  if (result.mode === 'explain') {
    return result.columns.length > 1 ? (
      <ResultGrid columns={result.columns} rows={result.rows} enhanced />
    ) : (
      <ExplainTree lines={result.rows.map((r) => r[0] ?? '')} />
    )
  }
  const exportAs = (kind: 'csv' | 'json') => {
    const text =
      kind === 'csv'
        ? toCsv(result.columns, result.rows)
        : toJson(result.columns, result.rows)
    downloadText(
      `${databaseName}-result.${kind}`,
      kind === 'csv' ? 'text/csv' : 'application/json',
      text,
    )
    toast.add({
      title: t('viewer.console.results.exported', {
        count: result.rows.length,
      }),
      type: 'success',
    })
  }
  return (
    <div className="space-y-2">
      {result.mode === 'write' ? (
        <p className="text-sm" role="status">
          {t('viewer.console.writeDone', { count: result.row_count })}
        </p>
      ) : null}
      {result.columns.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('viewer.console.okNoRows')}
        </p>
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-xs text-muted-foreground" role="status">
              {t('viewer.grid.rowCount', { count: result.row_count })} |{' '}
              {t('viewer.console.elapsed', { ms: result.duration_ms })}
              {result.truncated ? ` | ${t('viewer.grid.resultCapped')}` : ''}
            </p>
            <div className="flex items-center gap-1">
              {(['csv', 'json'] as const).map((kind) => (
                <Button
                  key={kind}
                  type="button"
                  size="sm"
                  variant="outline"
                  title={t('viewer.console.results.exportHint', {
                    count: result.rows.length,
                  })}
                  onClick={() => {
                    exportAs(kind)
                  }}
                >
                  <DownloadGlassIcon className="size-4" aria-hidden="true" />
                  {kind === 'csv'
                    ? t('viewer.console.results.exportCsv')
                    : t('viewer.console.results.exportJson')}
                </Button>
              ))}
            </div>
          </div>
          <ResultGrid columns={result.columns} rows={result.rows} enhanced />
        </>
      )}
    </div>
  )
}
