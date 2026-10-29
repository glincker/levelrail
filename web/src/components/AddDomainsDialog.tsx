import { Suspense, lazy, useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  ChevronRightGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  LoaderCircleGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { FormErrorSummary } from '@/components/ui/form-error-summary'
import { addDomainsIndependently, type BulkAddOutcome } from '../lib/domainBulk'
import {
  analyzeDomainList,
  createDomainListSchema,
  type DomainIssue,
} from '../lib/domainListSchema'
import { useAppForm } from '../hooks/useAppForm'
import { useEditAppDomains } from '../queries/appEnvironmentDomains'
import {
  domainCheckKeys,
  domainCheckQueryOptions,
} from '../queries/domainCheck'
import { GetCertificateButton } from './GetCertificateButton'
import type { EditDomainsResult } from '../queries/appEnvironmentDomains'
import { DomainCheckPanel } from './DomainDnsCheck'
import { DomainDnsStatusBadge } from './DomainDnsStatusBadge'
import { GoLivePanel } from './GoLivePanel'
import { ServerAddressLine } from './domains/ServerAddressCard'
import { StandaloneAddBody } from './domains/StandaloneAddBody'

const DomainDiagnosePanel = lazy(() =>
  import('./domains/DomainDiagnosePanel').then((m) => ({
    default: m.DomainDiagnosePanel,
  })),
)

export interface AddDomainsApp {
  name: string
}

function ResultRow({
  appName,
  outcome,
  diagnoseByDefault,
}: {
  appName: string
  outcome: BulkAddOutcome<EditDomainsResult>
  diagnoseByDefault: boolean
}) {
  const { t } = useTranslation('domains')
  const [open, setOpen] = useState(false)
  const [diagnoseOpen, setDiagnoseOpen] = useState(diagnoseByDefault)
  const check = useQuery({
    ...domainCheckQueryOptions(appName, outcome.domain),
    enabled: outcome.ok,
  })
  if (!outcome.ok) {
    return (
      <li className="flex items-start gap-2 py-2 text-sm">
        <CircleAlertGlassIcon
          className="mt-0.5 size-4.5 shrink-0 text-destructive"
          aria-hidden="true"
        />
        <div className="min-w-0">
          <p className="truncate font-mono">{outcome.domain}</p>
          <FieldError className="text-xs">{outcome.error}</FieldError>
        </div>
      </li>
    )
  }
  return (
    <li className="py-2 text-sm">
      <div className="flex items-center gap-2">
        <CircleCheckGlassIcon
          className="size-4.5 shrink-0 text-emerald-600"
          aria-hidden="true"
        />
        <span className="min-w-0 flex-1 truncate font-mono">
          {outcome.domain}
        </span>
        <DomainDnsStatusBadge status={check.data?.status} />
        <Button
          type="button"
          size="sm"
          variant="ghost"
          aria-expanded={open}
          onClick={() => {
            setOpen((v) => !v)
          }}
        >
          {open ? (
            <ChevronDownGlassIcon size="1.2em" />
          ) : (
            <ChevronRightGlassIcon size="1.2em" />
          )}
          {open
            ? t('page.addDialog.hideRecord')
            : t('page.addDialog.showRecord')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          aria-expanded={diagnoseOpen}
          onClick={() => {
            setDiagnoseOpen((v) => !v)
          }}
        >
          {t('page.row.diagnose')}
        </Button>
        <GetCertificateButton domain={outcome.domain} variant="ghost" />
      </div>
      <div className="mt-2 pl-6">
        <GoLivePanel
          app={appName}
          domain={outcome.domain}
          initial={outcome.result.go_live?.[0]}
          dns={outcome.result.dns_results?.[0]}
        />
      </div>
      {diagnoseOpen ? (
        <div className="mt-2 pl-6">
          <Suspense fallback={null}>
            <DomainDiagnosePanel domain={outcome.domain} />
          </Suspense>
        </div>
      ) : null}
      {open ? (
        <div className="mt-2">
          <DomainCheckPanel
            domain={outcome.domain}
            data={check.data}
            isFetching={check.isFetching}
            onRefetch={() => {
              void check.refetch()
            }}
          />
        </div>
      ) : null}
    </li>
  )
}

function AddDomainsBody({
  apps,
  claimedBy,
  initialApp,
  onClose,
}: {
  apps: AddDomainsApp[]
  claimedBy: ReadonlyMap<string, string>
  initialApp?: string
  onClose: () => void
}) {
  const { t } = useTranslation('domains')
  const queryClient = useQueryClient()
  const [outcomes, setOutcomes] = useState<
    BulkAddOutcome<EditDomainsResult>[] | null
  >(null)
  const [retrying, setRetrying] = useState(false)
  const [verifying, setVerifying] = useState(false)
  const schema = useMemo(() => createDomainListSchema(claimedBy), [claimedBy])
  const form = useAppForm(schema, {
    defaultValues: {
      app: initialApp ?? (apps.length === 1 ? (apps[0]?.name ?? '') : ''),
      domains: '',
    },
  })
  const {
    register,
    handleSubmit,
    setFocus,
    watch,
    setValue,
    formState: { errors, touchedFields, isSubmitted, isSubmitting },
  } = form
  const appName = watch('app')
  const text = watch('domains')
  const edit = useEditAppDomains(appName)
  const analysis = useMemo(
    () => analyzeDomainList(text, claimedBy),
    [text, claimedBy],
  )

  async function run(domains: string[]) {
    return addDomainsIndependently<EditDomainsResult>(domains, (domain) =>
      edit.mutateAsync({ add: [domain] }),
    )
  }

  const submit = handleSubmit(
    async (values) => {
      const { domains } = analyzeDomainList(values.domains, claimedBy)
      setOutcomes(await run(domains))
    },
    (invalid) => {
      setFocus(invalid.app ? 'app' : 'domains')
    },
  )

  async function retryFailed() {
    const previous = outcomes ?? []
    const failed = previous.filter((o) => !o.ok).map((o) => o.domain)
    setRetrying(true)
    const retried = await run(failed)
    setRetrying(false)
    setOutcomes([...previous.filter((o) => o.ok), ...retried])
  }

  async function verifyAll() {
    setVerifying(true)
    await Promise.all(
      (outcomes ?? [])
        .filter((o) => o.ok)
        .map((o) =>
          queryClient.refetchQueries({
            queryKey: domainCheckKeys.detail(appName, o.domain),
          }),
        ),
    )
    setVerifying(false)
  }

  if (apps.length === 0) {
    return (
      <>
        <DialogHeader>
          <DialogTitle>{t('page.addDialog.title')}</DialogTitle>
          <DialogDescription>{t('page.addDialog.noApps')}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button render={<Link to="/apps" />} nativeButton={false}>
            {t('page.empty.noAppsAction')}
          </Button>
        </DialogFooter>
      </>
    )
  }

  if (outcomes) {
    const okCount = outcomes.filter((o) => o.ok).length
    const failedCount = outcomes.length - okCount
    return (
      <>
        <DialogHeader>
          <DialogTitle>{t('page.addDialog.resultsTitle')}</DialogTitle>
          <DialogDescription>
            {t('page.addDialog.resultsBody')}
          </DialogDescription>
        </DialogHeader>
        <p className="text-xs text-muted-foreground">
          {t('page.addDialog.summary', { ok: okCount, failed: failedCount })}
        </p>
        <ul className="max-h-96 divide-y divide-border overflow-auto">
          {outcomes.map((o) => (
            <ResultRow
              key={o.domain}
              appName={appName}
              outcome={o}
              diagnoseByDefault={outcomes.length === 1}
            />
          ))}
        </ul>
        <DialogFooter>
          {failedCount > 0 ? (
            <Button
              type="button"
              variant="outline"
              disabled={retrying}
              onClick={() => {
                void retryFailed()
              }}
            >
              {retrying
                ? t('page.addDialog.submitting')
                : t('page.addDialog.retryFailed')}
            </Button>
          ) : null}
          {okCount > 0 ? (
            <Button
              type="button"
              variant="outline"
              disabled={verifying}
              onClick={() => {
                void verifyAll()
              }}
            >
              <RefreshCwGlassIcon
                size="1.2em"
                className={verifying ? 'animate-spin' : undefined}
              />
              {verifying
                ? t('page.addDialog.verifying')
                : t('page.addDialog.verify')}
            </Button>
          ) : null}
          <Button type="button" onClick={onClose}>
            {t('page.addDialog.done')}
          </Button>
        </DialogFooter>
      </>
    )
  }

  const showLineErrors = Boolean(touchedFields.domains) || isSubmitted
  const lineIssues = showLineErrors ? analysis.issues : []
  const domainsError = errors.domains?.message
  const emptyError = showLineErrors && domainsError === 'empty'
  const invalid = lineIssues.length > 0 || emptyError
  const canSubmit = appName !== '' && !isSubmitting

  function reasonText(issue: DomainIssue): string {
    if (issue.reason === 'claimed') {
      return issue.owner === appName
        ? t('page.addDialog.errors.claimedHere')
        : t('page.addDialog.errors.claimedOther', { app: issue.owner })
    }
    return t(`page.addDialog.errors.${issue.reason}`)
  }

  return (
    <form
      className="space-y-4"
      noValidate
      onSubmit={(e) => {
        void submit(e)
      }}
    >
      <DialogHeader>
        <DialogTitle>{t('page.addDialog.title')}</DialogTitle>
        <DialogDescription>{t('page.addDialog.description')}</DialogDescription>
      </DialogHeader>

      <FormErrorSummary
        form={form}
        fields={{
          app: { label: t('page.addDialog.app'), id: 'add-domains-app' },
          domains: {
            label: t('page.addDialog.domains'),
            id: 'add-domains-list',
          },
        }}
      />

      <Field data-invalid={Boolean(errors.app)}>
        <FieldLabel htmlFor="add-domains-app">
          {t('page.addDialog.app')}
        </FieldLabel>
        <Select
          value={appName || null}
          onValueChange={(value) => {
            setValue('app', value ?? '', { shouldValidate: isSubmitted })
          }}
        >
          <SelectTrigger id="add-domains-app" className="w-full">
            <SelectValue placeholder={t('page.addDialog.appPlaceholder')} />
          </SelectTrigger>
          <SelectContent>
            {apps.map((a) => (
              <SelectItem key={a.name} value={a.name}>
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {errors.app ? (
          <FieldError>{t('page.addDialog.errors.appRequired')}</FieldError>
        ) : null}
      </Field>

      <Field data-invalid={invalid}>
        <FieldLabel htmlFor="add-domains-list">
          {t('page.addDialog.domains')}
        </FieldLabel>
        <Textarea
          id="add-domains-list"
          className="min-h-28 font-mono"
          placeholder={t('page.addDialog.domainsPlaceholder')}
          {...register('domains')}
        />
        <FieldHint id="add-domains-hint">
          {t('page.addDialog.domainsHint')}
        </FieldHint>
        <div>
          {emptyError ? (
            <FieldError>{t('page.addDialog.errors.empty')}</FieldError>
          ) : null}
          {lineIssues.length > 0 ? (
            <FieldError>
              <ul className="space-y-0.5">
                {lineIssues.map((issue, i) => (
                  <li key={`${issue.value}-${i}`}>
                    <span className="font-mono">{issue.value}</span>
                    <span className="sr-only">{': '}</span>
                    <span className="ml-2">{reasonText(issue)}</span>
                  </li>
                ))}
              </ul>
            </FieldError>
          ) : null}
        </div>
      </Field>

      <p className="text-xs text-muted-foreground" aria-live="polite">
        {t('page.addDialog.ready', { count: analysis.domains.length })}
      </p>

      {edit.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{edit.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <DialogFooter>
        <Button type="submit" disabled={!canSubmit}>
          {isSubmitting ? (
            <LoaderCircleGlassIcon
              size="1.2em"
              className="animate-spin"
              aria-hidden="true"
            />
          ) : null}
          {isSubmitting
            ? t('page.addDialog.submitting')
            : t('page.addDialog.submit', { count: analysis.domains.length })}
        </Button>
      </DialogFooter>
    </form>
  )
}

type AddMode = 'app' | 'proxy'

function ModeSwitch({
  mode,
  onMode,
}: {
  mode: AddMode
  onMode: (m: AddMode) => void
}) {
  const { t } = useTranslation('domains')
  return (
    <div
      role="radiogroup"
      aria-label={t('standalone.modeLabel')}
      className="grid grid-cols-2 gap-1 rounded-lg bg-muted p-1"
    >
      {(['app', 'proxy'] as const).map((m) => (
        <Button
          key={m}
          type="button"
          size="sm"
          role="radio"
          aria-checked={mode === m}
          variant={mode === m ? 'outline' : 'ghost'}
          onClick={() => {
            onMode(m)
          }}
        >
          {m === 'app' ? t('standalone.modeApp') : t('standalone.modeProxy')}
        </Button>
      ))}
    </div>
  )
}

function AddDomainsModes({
  apps,
  claimedBy,
  initialApp,
  onClose,
}: {
  apps: AddDomainsApp[]
  claimedBy: ReadonlyMap<string, string>
  initialApp?: string
  onClose: () => void
}) {
  const [mode, setMode] = useState<AddMode>(
    apps.length === 0 && !initialApp ? 'proxy' : 'app',
  )
  return (
    <>
      {initialApp ? null : <ModeSwitch mode={mode} onMode={setMode} />}
      <ServerAddressLine />
      {mode === 'proxy' ? (
        <StandaloneAddBody onClose={onClose} />
      ) : (
        <AddDomainsBody
          apps={apps}
          claimedBy={claimedBy}
          initialApp={initialApp}
          onClose={onClose}
        />
      )}
    </>
  )
}

export function AddDomainsDialog({
  open,
  onOpenChange,
  apps,
  claimedBy,
  initialApp,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  apps: AddDomainsApp[]
  claimedBy: ReadonlyMap<string, string>
  initialApp?: string
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        {open ? (
          <AddDomainsModes
            apps={apps}
            claimedBy={claimedBy}
            initialApp={initialApp}
            onClose={() => {
              onOpenChange(false)
            }}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
