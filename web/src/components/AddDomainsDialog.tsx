import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  CaretDownIcon,
  CaretRightIcon,
  CheckCircleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  addDomainsIndependently,
  parseDomainInput,
  type BulkAddOutcome,
} from '../lib/domainBulk'
import { useEditAppDomains } from '../queries/appEnvironmentDomains'
import {
  domainCheckKeys,
  domainCheckQueryOptions,
} from '../queries/domainCheck'
import { DomainCheckPanel } from './DomainDnsCheck'
import { DomainDnsStatusBadge } from './DomainDnsStatusBadge'

export interface AddDomainsApp {
  name: string
}

function ResultRow({
  appName,
  outcome,
}: {
  appName: string
  outcome: BulkAddOutcome
}) {
  const { t } = useTranslation('domains')
  const [open, setOpen] = useState(false)
  const check = useQuery({
    ...domainCheckQueryOptions(appName, outcome.domain),
    enabled: outcome.ok,
  })
  if (!outcome.ok) {
    return (
      <li className="flex items-start gap-2 py-2 text-sm">
        <WarningCircleIcon
          className="mt-0.5 size-4 shrink-0 text-destructive"
          aria-hidden="true"
        />
        <div className="min-w-0">
          <p className="truncate font-mono">{outcome.domain}</p>
          <p className="text-xs text-destructive">{outcome.error}</p>
        </div>
      </li>
    )
  }
  return (
    <li className="py-2 text-sm">
      <div className="flex items-center gap-2">
        <CheckCircleIcon
          className="size-4 shrink-0 text-emerald-600"
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
          {open ? <CaretDownIcon /> : <CaretRightIcon />}
          {open
            ? t('page.addDialog.hideRecord')
            : t('page.addDialog.showRecord')}
        </Button>
      </div>
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
  const [appName, setAppName] = useState(
    initialApp ?? (apps.length === 1 ? (apps[0]?.name ?? '') : ''),
  )
  const [text, setText] = useState('')
  const [outcomes, setOutcomes] = useState<BulkAddOutcome[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [verifying, setVerifying] = useState(false)
  const edit = useEditAppDomains(appName)
  const parsed = useMemo(
    () => parseDomainInput(text, claimedBy),
    [text, claimedBy],
  )

  async function run(domains: string[]) {
    setBusy(true)
    const results = await addDomainsIndependently(domains, (domain) =>
      edit.mutateAsync({ add: [domain] }),
    )
    setBusy(false)
    return results
  }

  async function submit() {
    setOutcomes(await run(parsed.add))
  }

  async function retryFailed() {
    const previous = outcomes ?? []
    const failed = previous.filter((o) => !o.ok).map((o) => o.domain)
    const retried = await run(failed)
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
        <ul className="max-h-80 divide-y divide-border overflow-auto">
          {outcomes.map((o) => (
            <ResultRow key={o.domain} appName={appName} outcome={o} />
          ))}
        </ul>
        <DialogFooter>
          {failedCount > 0 ? (
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={() => {
                void retryFailed()
              }}
            >
              {t('page.addDialog.retryFailed')}
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
              <ArrowsClockwiseIcon
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

  const canSubmit = appName !== '' && parsed.add.length > 0 && !busy
  return (
    <>
      <DialogHeader>
        <DialogTitle>{t('page.addDialog.title')}</DialogTitle>
        <DialogDescription>{t('page.addDialog.description')}</DialogDescription>
      </DialogHeader>

      <Field>
        <FieldLabel htmlFor="add-domains-app">
          {t('page.addDialog.app')}
        </FieldLabel>
        <Select
          value={appName || null}
          onValueChange={(value) => {
            setAppName(value ?? '')
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
      </Field>

      <Field>
        <FieldLabel htmlFor="add-domains-list">
          {t('page.addDialog.domains')}
        </FieldLabel>
        <Textarea
          id="add-domains-list"
          className="min-h-28 font-mono"
          value={text}
          placeholder={t('page.addDialog.domainsPlaceholder')}
          onChange={(e) => {
            setText(e.target.value)
          }}
        />
        <FieldHint>{t('page.addDialog.domainsHint')}</FieldHint>
      </Field>

      <div className="space-y-1 text-xs" aria-live="polite">
        {parsed.add.length > 0 ? (
          <p className="text-muted-foreground">
            {t('page.addDialog.ready', { count: parsed.add.length })}
          </p>
        ) : null}
        {parsed.invalid.length > 0 ? (
          <p className="text-destructive">
            {t('page.addDialog.invalid', { list: parsed.invalid.join(', ') })}
          </p>
        ) : null}
        {parsed.duplicates.length > 0 ? (
          <p className="text-muted-foreground">
            {t('page.addDialog.duplicates', {
              list: parsed.duplicates.join(', '),
            })}
          </p>
        ) : null}
        {parsed.claimed.map((c) => (
          <p key={c.domain} className="text-amber-700 dark:text-amber-400">
            {t('page.addDialog.claimed', { domain: c.domain, app: c.app })}
          </p>
        ))}
      </div>

      {edit.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{edit.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <DialogFooter>
        <Button
          type="button"
          disabled={!canSubmit}
          onClick={() => void submit()}
        >
          {busy
            ? t('page.addDialog.submitting')
            : t('page.addDialog.submit', { count: parsed.add.length })}
        </Button>
      </DialogFooter>
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
      <DialogContent className="sm:max-w-lg">
        {open ? (
          <AddDomainsBody
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
