import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PlayGlassIcon,
  SaveGlassIcon,
  TriangleAlertGlassIcon,
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { CodeBlock } from '@/components/ui/code-block'
import { toast } from '@/components/ui/toast'
import { useWriteUnlock } from '../../hooks/useWriteUnlock'
import { confirmName, detectDestructive } from '../../lib/destructiveSql'
import { WriteUnlockDialog } from './WriteUnlockDialog'
import { useRunDatabaseQuery, useSaveQuery } from '../../queries/databaseViewer'
import type { DbQueryResponse } from '../../types/databaseViewer'
import { HistoryPanel, SavedPanel } from './ConsoleSidePanels'
import { ResultGrid } from './ResultGrid'

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
  const [sql, setSql] = useState(initialSql)
  const unlock = useWriteUnlock(databaseName)
  const writes = unlock.unlocked
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [destructiveOpen, setDestructiveOpen] = useState(false)
  const [typedObject, setTypedObject] = useState('')
  const [saveOpen, setSaveOpen] = useState(false)
  const [saveName, setSaveName] = useState('')
  const [result, setResult] = useState<DbQueryResponse | null>(null)
  const run = useRunDatabaseQuery()
  const save = useSaveQuery(databaseName)

  const canRun = sql.trim() !== '' && !run.isPending

  function execute(mode: 'read' | 'write' | 'explain', analyze = false) {
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
          setResult(data)
        },
        onError: () => {
          setResult(null)
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

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-end gap-3">
        <label className="flex items-center gap-2 text-sm">
          <Switch checked={writes} onCheckedChange={onToggleWrites} />
          <span>{t('viewer.console.writeToggle')}</span>
        </label>
      </div>
      {writes ? null : (
        <p className="text-xs text-muted-foreground">
          {t('viewer.console.writeToggleHint')}
        </p>
      )}

      <div className="space-y-2">
        <Textarea
          value={sql}
          aria-label={t('viewer.console.editorLabel')}
          placeholder={t('viewer.console.placeholder')}
          spellCheck={false}
          className="min-h-40 font-mono text-sm"
          onChange={(e) => {
            setSql(e.target.value)
          }}
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && canRun) {
              e.preventDefault()
              runClicked()
            }
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
            onClick={() => {
              setSaveName('')
              setSaveOpen(true)
            }}
          >
            <SaveGlassIcon className="size-5" aria-hidden="true" />
            {t('viewer.console.saved.save')}
          </Button>
          <span className="text-xs text-muted-foreground">
            {t('viewer.console.shortcut')}
          </span>
        </div>
      </div>

      <Tabs defaultValue="results">
        <TabsList>
          <TabsTrigger value="results">
            {t('viewer.console.tabResults')}
          </TabsTrigger>
          <TabsTrigger value="history">
            {t('viewer.console.tabHistory')}
          </TabsTrigger>
          <TabsTrigger value="saved">
            {t('viewer.console.tabSaved')}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="results" className="space-y-2">
          <ResultsView
            result={result}
            error={run.error?.message ?? null}
            pending={run.isPending}
          />
        </TabsContent>
        <TabsContent value="history">
          <HistoryPanel databaseName={databaseName} onLoad={setSql} />
        </TabsContent>
        <TabsContent value="saved">
          <SavedPanel databaseName={databaseName} onLoad={setSql} />
        </TabsContent>
      </Tabs>

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
  result,
  error,
  pending,
}: {
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
      <p className="text-sm text-muted-foreground">
        {pending
          ? t('viewer.console.running')
          : t('viewer.console.noResultsYet')}
      </p>
    )
  }
  if (result.mode === 'explain') {
    return (
      <div className="space-y-1">
        <p className="text-xs font-medium text-muted-foreground">
          {t('viewer.console.plan')}
        </p>
        <CodeBlock
          code={result.rows.map((r) => r[0] ?? '').join('\n')}
          maxHeight="lg"
        />
      </div>
    )
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
        <ResultGrid columns={result.columns} rows={result.rows} />
      )}
      <p className="text-xs text-muted-foreground">
        {t('viewer.grid.rowCount', { count: result.row_count })} |{' '}
        {t('viewer.console.elapsed', { ms: result.duration_ms })}
        {result.truncated ? ` | ${t('viewer.grid.resultCapped')}` : ''}
      </p>
    </div>
  )
}
