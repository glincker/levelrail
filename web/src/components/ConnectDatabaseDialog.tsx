import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftIcon,
  PlugsConnectedIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useAdoptExternalDatabase,
  useConnectExternalDatabase,
  useExternalCandidates,
  useTestExternalDatabase,
} from '../queries/externalDatabases'
import type {
  ExternalEngine,
  ExternalProbeResult,
} from '../types/externalDatabase'
import { ConnectDatabaseDetails } from './ConnectDatabaseDetails'
import {
  DEFAULT_PORT,
  EMPTY_FORM,
  defaultTls,
  formFromCandidate,
  type FormState,
  type Step,
} from './connectDatabaseForm'

// Guided dialog for connecting a database that already runs somewhere:
// adopt a local container, or enter an address. A connection test runs
// before saving; nothing is moved or changed on the database.
export function ConnectDatabaseDialog({
  trigger,
}: {
  trigger?: React.ReactElement
}) {
  const { t } = useTranslation('databases')
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [step, setStep] = useState<Step>('source')
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [probe, setProbe] = useState<ExternalProbeResult | null>(null)

  const candidates = useExternalCandidates('', open && step === 'pick')
  const test = useTestExternalDatabase()
  const connect = useConnectExternalDatabase()
  const adopt = useAdoptExternalDatabase()
  const adopting = form.container !== ''
  const saving = connect.isPending || adopt.isPending
  const saveError = connect.error ?? adopt.error

  function reset() {
    setStep('source')
    setForm(EMPTY_FORM)
    setProbe(null)
    test.reset()
    connect.reset()
    adopt.reset()
  }

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset()
    }
  }

  function patch(p: Partial<FormState>) {
    setForm((f) => ({ ...f, ...p }))
    setProbe(null)
  }

  function setEngine(engine: ExternalEngine) {
    patch({
      engine,
      port: String(DEFAULT_PORT[engine]),
      tls: defaultTls(engine),
    })
  }

  function request() {
    return {
      name: form.name,
      engine: form.engine,
      host: form.host,
      port: Number(form.port) || undefined,
      network: form.network || undefined,
      username: form.username || undefined,
      password: form.password || undefined,
      database: form.database || undefined,
      tls_mode: form.tls,
      container: adopting ? form.container : undefined,
    }
  }

  function runTest() {
    test.mutate(request(), { onSuccess: setProbe })
  }

  function save() {
    const mutation = adopting ? adopt : connect
    mutation.mutate(request(), {
      onSuccess: (created) => {
        setOpen(false)
        reset()
        toast.add({
          title: t('external.connect.toast', { name: created.name }),
          type: 'success',
        })
        void navigate({
          to: '/databases/$name/overview',
          params: { name: created.name },
        })
      },
    })
  }

  const canSave = form.name !== '' && form.host !== '' && !saving

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={trigger ?? <Button size="sm" variant="outline" />}>
        <PlugsConnectedIcon className="size-4" aria-hidden="true" />
        {t('external.connect.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('external.connect.title')}</DialogTitle>
          <DialogDescription>
            {t('external.connect.description')}
          </DialogDescription>
        </DialogHeader>

        {step === 'source' ? (
          <div className="grid gap-2">
            <button
              type="button"
              className="rounded-lg border border-border p-3 text-left transition-colors hover:bg-muted/60"
              onClick={() => {
                setStep('pick')
              }}
            >
              <span className="block text-sm font-medium">
                {t('external.connect.sourceAdopt')}
              </span>
              <span className="block text-xs text-muted-foreground">
                {t('external.connect.sourceAdoptHint')}
              </span>
            </button>
            <button
              type="button"
              className="rounded-lg border border-border p-3 text-left transition-colors hover:bg-muted/60"
              onClick={() => {
                setStep('details')
              }}
            >
              <span className="block text-sm font-medium">
                {t('external.connect.sourceManual')}
              </span>
              <span className="block text-xs text-muted-foreground">
                {t('external.connect.sourceManualHint')}
              </span>
            </button>
          </div>
        ) : null}

        {step === 'pick' ? (
          <div className="space-y-2">
            {candidates.isLoading ? (
              <p className="text-sm text-muted-foreground">
                {t('external.connect.loadingCandidates')}
              </p>
            ) : null}
            {candidates.isError ? (
              <p className="text-sm text-destructive" role="alert">
                {t('external.connect.candidatesError')}
              </p>
            ) : null}
            {candidates.data?.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('external.connect.noCandidates')}
              </p>
            ) : null}
            {candidates.data?.map((c) => (
              <button
                key={c.container_id}
                type="button"
                className="flex w-full items-center justify-between gap-3 rounded-lg border border-border p-3 text-left transition-colors hover:bg-muted/60"
                onClick={() => {
                  setForm(formFromCandidate(c))
                  setStep('details')
                }}
              >
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">
                    {c.container}
                  </span>
                  <span className="block truncate font-mono text-xs text-muted-foreground">
                    {c.image}
                  </span>
                  {c.note ? (
                    <span className="block text-xs text-muted-foreground">
                      {c.note}
                    </span>
                  ) : null}
                </span>
                <span className="font-mono text-xs text-muted-foreground">
                  {c.network ?? ''}
                </span>
              </button>
            ))}
          </div>
        ) : null}

        {step === 'details' ? (
          <ConnectDatabaseDetails
            form={form}
            adopting={adopting}
            probe={probe}
            testing={test.isPending}
            testError={test.isError ? test.error.message : null}
            saveError={saveError ? saveError.message : null}
            patch={patch}
            setEngine={setEngine}
            setDatabase={(d) => {
              setForm((f) => ({ ...f, database: d }))
            }}
            runTest={runTest}
          />
        ) : null}

        <DialogFooter>
          {step !== 'source' ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                setStep(
                  step === 'details' && !adopting
                    ? 'source'
                    : step === 'details'
                      ? 'pick'
                      : 'source',
                )
              }}
            >
              <ArrowLeftIcon className="size-3.5" aria-hidden="true" />
              {t('external.connect.back')}
            </Button>
          ) : null}
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            {t('external.connect.cancel')}
          </Button>
          {step === 'details' ? (
            <Button type="button" disabled={!canSave} onClick={save}>
              {saving
                ? t('external.connect.saving')
                : t('external.connect.save')}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
