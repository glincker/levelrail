import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  FloppyDiskIcon,
  LockSimpleIcon,
  PlayIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
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
import { toast } from '@/components/ui/toast'
import { useRunDatabaseQuery, useSaveQuery } from '../../queries/databaseViewer'
import type { DbQueryResponse } from '../../types/databaseViewer'
import { HistoryPanel, SavedPanel } from './ConsoleSidePanels'
import { ResultGrid } from './ResultGrid'

// SQL console. Read-only unless an admin explicitly enables writes for
// this page session; the server enforces both (read-only transaction,
// statement guard, admin-only route), this UI only mirrors it.
export function SqlConsole({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('databases')
  const [sql, setSql] = useState('')
  const [writes, setWrites] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [confirmText, setConfirmText] = useState('')
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

  function onToggleWrites(next: boolean) {
    if (next) {
      setConfirmText('')
      setConfirmOpen(true)
      return
    }
    setWrites(false)
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          {writes ? (
            <Badge variant="destructive">
              <WarningIcon className="size-3" aria-hidden="true" />
              {t('viewer.console.writesOn')}
            </Badge>
          ) : (
            <Badge variant="muted">
              <LockSimpleIcon className="size-3" aria-hidden="true" />
              {t('viewer.console.readOnly')}
            </Badge>
          )}
        </div>
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
              execute('read')
            }
          }}
        />
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            size="sm"
            disabled={!canRun}
            onClick={() => {
              execute('read')
            }}
          >
            <PlayIcon className="size-3.5" aria-hidden="true" />
            {run.isPending
              ? t('viewer.console.running')
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
            <FloppyDiskIcon className="size-3.5" aria-hidden="true" />
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

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-1.5 text-destructive">
              <WarningIcon className="size-4" aria-hidden="true" />
              {t('viewer.console.writeConfirmTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('viewer.console.writeConfirmDescription', {
                name: databaseName,
              })}
            </DialogDescription>
          </DialogHeader>
          <label className="space-y-1 text-sm">
            <span>{t('viewer.console.writeConfirmLabel')}</span>
            <Input
              value={confirmText}
              autoComplete="off"
              onChange={(e) => {
                setConfirmText(e.target.value)
              }}
            />
          </label>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setConfirmOpen(false)
              }}
            >
              {t('viewer.console.cancel')}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={confirmText !== databaseName}
              onClick={() => {
                setWrites(true)
                setConfirmOpen(false)
              }}
            >
              {t('viewer.console.writeConfirmAction')}
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
        <pre className="max-h-96 overflow-auto rounded-md bg-muted p-3 font-mono text-xs">
          {result.rows.map((r) => r[0] ?? '').join('\n')}
        </pre>
      </div>
    )
  }
  return (
    <div className="space-y-2">
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
