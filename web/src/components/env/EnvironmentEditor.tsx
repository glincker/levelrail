import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ListGlassIcon, RefreshCwGlassIcon } from '@thesvg/ui-react'
import type { AppDetail } from '../../types/appDetail'
import { useRestartApp } from '../../queries/apps'
import {
  EnvApiError,
  exportEnv,
  revealValue,
  useConvertToSecret,
  useEnvSettings,
  useUpdateEnvSettings,
  type ExportFormat,
} from '../../queries/envProtection'
import { useEnvEditorState } from '../../hooks/useEnvEditorState'
import { useRevealedValues } from '../../hooks/useRevealedValues'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  applyDotenv,
  MASKED_VALUE,
  rowsToDotenv,
  type EnvRow,
} from '../../lib/envRows'
import { HelpLink } from '../HelpLink'
import { BoundEnvSection } from './BoundEnvSection'
import { AddVariableRow } from './AddVariableRow'
import { EnvEditorToolbar } from './EnvEditorToolbar'
import { EnvPolicyNotice } from './EnvPolicyNotice'
import { EnvRowList } from './EnvRowList'
import { ENV_ROW_GRID, EnvVarRow } from './EnvVarRow'
import { ReviewChangesDialog } from './ReviewChangesDialog'
import { StepUpDialog } from './StepUpDialog'
import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'

function download(name: string, text: string, json: boolean) {
  const blob = new Blob([text], {
    type: json ? 'application/json' : 'text/plain',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

export function EnvironmentEditor({ app }: { app: AppDetail }) {
  const { t } = useTranslation('envEditor')
  const state = useEnvEditorState(app)
  const settings = useEnvSettings(app.name)
  const revealed = useRevealedValues()
  const updateSettings = useUpdateEnvSettings(app.name)
  const convert = useConvertToSecret(app.name)
  const restartApp = useRestartApp()
  const [mode, setMode] = useState<'table' | 'bulk'>('table')
  const [bulkText, setBulkText] = useState('')
  const [adding, setAdding] = useState<{ key?: string } | null>(null)
  const [reviewOpen, setReviewOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [convertKey, setConvertKey] = useState<string | null>(null)

  const showError = useCallback(
    (error: unknown) => {
      let title = error instanceof Error ? error.message : String(error)
      if (error instanceof EnvApiError && error.code === 'reveal_disabled')
        title = t('errors.revealDisabled')
      if (error instanceof EnvApiError && error.code === 'app_write_only')
        title = t('errors.writeOnly')
      toast.add({ title, type: 'error' })
    },
    [t],
  )
  const gate = useStepUpGate(showError)

  const policy = settings.data
  const writeOnly =
    policy?.secrets_write_only ?? app.secrets_write_only ?? false
  const canReveal =
    policy !== undefined &&
    policy.policy_valid &&
    policy.reveal_policy !== 'disabled' &&
    !writeOnly
  const secretsConfigured = policy?.secrets_configured ?? true
  const scope = (action: 'reveal' | 'export' | 'manage') => ({
    action,
    app: app.name,
  })

  const ownKeys = useMemo(
    () =>
      new Set(
        state.rows
          .filter((r) => r.origin === 'own' && !r.removed)
          .map((r) => r.key),
      ),
    [state.rows],
  )
  const ownRows = state.rows.filter((r) => r.origin === 'own')
  const scanVars = useMemo(() => {
    const out: Record<string, string> = {}
    const touched = new Set(
      state.changes.filter((c) => c.kind !== 'removed').map((c) => c.key),
    )
    for (const r of state.rows) {
      if (
        r.origin === 'own' &&
        !r.removed &&
        r.type === 'plain' &&
        r.value !== MASKED_VALUE &&
        touched.has(r.key)
      )
        out[r.key] = r.value
    }
    return out
  }, [state.rows, state.changes])
  const storeAsSecrets = useCallback(
    (keys: string[]) => {
      const set = new Set(keys)
      state.replaceRows(
        state.rows.map((r) =>
          r.origin === 'own' && r.type === 'plain' && set.has(r.key)
            ? { ...r, type: 'secret' as const }
            : r,
        ),
      )
    },
    [state],
  )
  const inheritedRows = state.rows.filter(
    (r) => r.origin === 'inherited' && !ownKeys.has(r.key),
  )

  const reveal = (row: EnvRow) => {
    void gate.guard(
      scope('reveal'),
      async () => {
        const res = await revealValue(app.name, row.key, row.type === 'secret')
        revealed.reveal(row.key, res.value, res.hide_after_seconds)
      },
      row.key,
    )
  }

  const runExport = (format: ExportFormat, full: boolean) => {
    const file = `${app.name}${format === 'json' ? '.env.json' : '.env'}`
    const run = async () => {
      download(file, await exportEnv(app.name, format, full), format === 'json')
    }
    if (!full) {
      run().catch(showError)
      return
    }
    void gate.guard(scope('export'), run)
  }

  const setSensitivity = (key: string, value: boolean | null) => {
    void gate.guard(scope('manage'), async () => {
      await updateSettings.mutateAsync({
        sensitive_overrides: { [key]: value },
      })
    })
  }

  const setWriteOnly = (value: boolean) => {
    void gate.guard(scope('manage'), async () => {
      await updateSettings.mutateAsync({ secrets_write_only: value })
      if (value) revealed.hideAll()
    })
  }

  const toggleMode = () => {
    if (mode === 'table') {
      setBulkText(rowsToDotenv(state.rows, revealed.values))
      setMode('bulk')
      return
    }
    state.replaceRows(applyDotenv(state.rows, bulkText, revealed.values))
    setMode('table')
  }

  const importFile = (text: string) => {
    const merged = `${rowsToDotenv(state.rows, revealed.values)}\n${text}`
    state.replaceRows(applyDotenv(state.rows, merged, revealed.values))
  }

  const save = async () => {
    setSaving(true)
    setSaveError(null)
    try {
      await state.save()
      setReviewOpen(false)
      toast.add({ title: t('saved'), type: 'success' })
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : String(error))
    } finally {
      setSaving(false)
    }
  }

  const copy = (value: string) => {
    void navigator.clipboard.writeText(value).then(() => {
      toast.add({ title: t('actions.copied'), type: 'success' })
    })
  }

  const renderRow = (row: EnvRow) => (
    <EnvVarRow
      row={row}
      shadows={row.origin === 'own' ? state.inheritedTiers[row.key] : undefined}
      revealed={revealed.values[row.key]}
      expiresAt={revealed.expiresAt[row.key]}
      canReveal={canReveal}
      canConvert={secretsConfigured && row.sensitive}
      onChange={state.updateRow}
      onReveal={() => {
        reveal(row)
      }}
      onHide={() => {
        revealed.hide(row.key)
      }}
      onCopy={copy}
      onOverride={() => {
        setAdding({ key: row.key })
      }}
      onSensitivity={(value) => {
        setSensitivity(row.key, value)
      }}
      onConvert={() => {
        setConvertKey(row.key)
      }}
    />
  )

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-2">
          <div className="space-y-1">
            <CardTitle>{t('title')}</CardTitle>
            <CardDescription>{t('description')}</CardDescription>
          </div>
          <HelpLink path="/environment-and-secrets" label={t('guide')} />
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <EnvEditorToolbar
          mode={mode}
          dirty={state.dirty}
          canFullExport={canReveal}
          writeOnly={writeOnly}
          writeOnlyPending={updateSettings.isPending}
          onAdd={() => {
            setAdding({})
          }}
          onToggleMode={toggleMode}
          onImport={importFile}
          onExport={runExport}
          onWriteOnly={setWriteOnly}
          onReview={() => {
            setSaveError(null)
            setReviewOpen(true)
          }}
          onDiscard={state.discard}
        />
        <EnvPolicyNotice settings={policy} />
        {app.env_dirty ? (
          <Alert>
            <RefreshCwGlassIcon size="1.4em" aria-hidden />
            <AlertTitle>{t('redeploy.title')}</AlertTitle>
            <AlertDescription>{t('redeploy.description')}</AlertDescription>
            <AlertAction>
              <Button
                type="button"
                size="sm"
                writeAction
                disabled={restartApp.isPending}
                onClick={() => {
                  restartApp.mutate(app.name, {
                    onSuccess: () =>
                      toast.add({
                        title: t('redeploy.restarting', { name: app.name }),
                        type: 'success',
                      }),
                    onError: showError,
                  })
                }}
              >
                {t('redeploy.restart')}
              </Button>
            </AlertAction>
          </Alert>
        ) : null}

        {mode === 'bulk' ? (
          <div className="space-y-2">
            <Textarea
              aria-label={t('bulk.label')}
              className="min-h-64 font-mono"
              spellCheck={false}
              value={bulkText}
              onChange={(e) => {
                setBulkText(e.target.value)
              }}
            />
            <p className="text-xs text-muted-foreground">{t('bulk.hint')}</p>
            <Button type="button" size="sm" onClick={toggleMode}>
              {t('bulk.apply')}
            </Button>
          </div>
        ) : (
          <div className="overflow-hidden rounded-lg border border-border">
            <div
              className={`${ENV_ROW_GRID} hidden border-b border-border bg-muted/40 text-xs font-medium text-muted-foreground sm:grid`}
            >
              <span>{t('columns.key')}</span>
              <span>{t('columns.value')}</span>
              <span>{t('columns.type')}</span>
              <span>{t('columns.scope')}</span>
              <span className="sr-only">{t('columns.actions')}</span>
            </div>
            {adding ? (
              <AddVariableRow
                key={adding.key ?? 'new'}
                existingKeys={ownKeys}
                secretsConfigured={secretsConfigured}
                initialKey={adding.key}
                onAdd={(row) => {
                  state.addRow(row)
                  setAdding(null)
                }}
                onCancel={() => {
                  setAdding(null)
                }}
              />
            ) : null}
            {ownRows.length === 0 && !adding ? (
              <EmptyState
                icon={<ListGlassIcon size="1.4em" />}
                title={t('empty.title')}
                description={t('empty.description')}
              />
            ) : (
              <EnvRowList
                rows={ownRows}
                label={t('title')}
                renderRow={renderRow}
              />
            )}
            {inheritedRows.length > 0 ? (
              <section
                aria-label={t('inherited.title')}
                className="border-t border-border"
              >
                <div className="px-3 pt-3">
                  <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
                    {t('inherited.title')}
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    {t('inherited.description')}
                  </p>
                </div>
                <EnvRowList
                  rows={inheritedRows}
                  label={t('inherited.title')}
                  renderRow={renderRow}
                />
              </section>
            ) : null}
          </div>
        )}
        <BoundEnvSection app={app.name} />
      </CardContent>

      <ReviewChangesDialog
        open={reviewOpen}
        changes={state.changes}
        saving={saving}
        error={saveError}
        scanApp={app.name}
        scanVars={scanVars}
        onStoreAsSecrets={storeAsSecrets}
        onConfirm={() => {
          void save()
        }}
        onOpenChange={setReviewOpen}
      />
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        subject={gate.pending?.subject}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
      <ConfirmDialog
        open={convertKey !== null}
        onOpenChange={(open) => {
          if (!open) setConvertKey(null)
        }}
        title={t('convert.title', { key: convertKey ?? '' })}
        description={t('convert.description')}
        consequences={[t('convert.consequence')]}
        confirmLabel={t('convert.confirm')}
        pendingLabel={t('convert.pending')}
        pending={convert.isPending}
        error={convert.error ? convert.error.message : null}
        onConfirm={() => {
          if (!convertKey) return
          const key = convertKey
          convert.mutate(key, {
            onSuccess: () => {
              setConvertKey(null)
              toast.add({ title: t('convert.done', { key }), type: 'success' })
            },
          })
        }}
      />
    </Card>
  )
}
