import { useCallback, useMemo, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  EllipsisGlassIcon,
  InboxGlassIcon,
  PlusGlassIcon,
  SearchGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { useCanWriteSensitive } from '../../hooks/useCanWriteSensitive'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  claimMailDomain,
  createMailbox,
  deleteMailbox,
  mailStepUpScope,
  resetMailboxPassword,
  setMailboxSuspended,
  useInvalidateMail,
  useMailboxes,
  type MailDomain,
  type Mailbox,
} from '../../queries/mailMailboxes'
import { useMailStatus } from '../../queries/mailStatus'
import { useSystemEmail } from '../../queries/mailSystemEmail'
import { teamListQueryOptions } from '../../queries/teams'
import { userListQueryOptions } from '../../queries/users'
import { StepUpDialog } from '../env/StepUpDialog'
import { MailMailboxSheet } from './MailMailboxSheet'
import { MailSecretReveal } from './MailSecretReveal'
import { FIELD_CLASS } from './mailPanelUi'

type Filter = 'all' | 'active' | 'suspended'

interface Props {
  domains: MailDomain[]
  adding: boolean
  onAdding: (v: boolean) => void
}

/** Dense mailbox list: search, status filter, on-demand add row and a detail sheet. */
export function MailMailboxes({ domains, adding, onAdding }: Props) {
  const { t } = useTranslation('mailServer')
  const status = useMailStatus().data
  const boxesQ = useMailboxes()
  const boxes = useMemo(() => boxesQ.data ?? [], [boxesQ.data])
  const teams = useQuery(teamListQueryOptions()).data
  const users = useQuery(userListQueryOptions()).data
  const canWrite = useCanWriteSensitive()
  const isRoot = useIsRoot()
  const sysFrom = useSystemEmail(isRoot).data?.from_address
  const invalidate = useInvalidateMail()
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [name, setName] = useState('')
  const [domain, setDomain] = useState('')
  const [linked, setLinked] = useState('')
  const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<Mailbox | null>(null)
  const [secret, setSecret] = useState<{
    address: string
    password: string
  } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const onError = useCallback(
    (e: unknown) =>
      setError(e instanceof Error ? e.message : t('mailboxes.failed')),
    [t],
  )
  const gate = useStepUpGate(onError)

  const install = status?.domain ?? ''
  const claimedNames = domains.map((d) => d.domain)
  const choices =
    install && !claimedNames.includes(install)
      ? [install, ...claimedNames]
      : claimedNames
  const chosen = domain || choices[0] || ''
  const needsClaim = !!chosen && !claimedNames.includes(chosen)
  const teamId = teams?.find((x) => x.default)?.id ?? teams?.[0]?.id ?? ''
  const teamName = (id: string) => teams?.find((x) => x.id === id)?.name ?? id
  const userName = (id?: string) =>
    id ? (users?.find((u) => u.id === id)?.email ?? id) : t('mailboxes.noUser')
  const writeWhy = canWrite ? undefined : t('mailboxes.needsWrite')

  const shown = useMemo(() => {
    const term = q.trim().toLowerCase()
    return boxes.filter((m) => {
      if (term && !m.address.toLowerCase().includes(term)) return false
      if (filter === 'active') return !m.suspended && !m.suspend_pending
      if (filter === 'suspended') return m.suspended || m.suspend_pending
      return true
    })
  }, [boxes, q, filter])
  const current = boxes.find((m) => m.address === open) ?? null

  const toggle = (m: Mailbox) =>
    gate.guard(mailStepUpScope.password(m.address), async () => {
      await setMailboxSuspended(m.address, !m.suspended)
      void invalidate()
    })
  const reset = (m: Mailbox) =>
    gate.guard(mailStepUpScope.password(m.address), async () => {
      const res = await resetMailboxPassword(m.address)
      setSecret({ address: m.address, password: res.password })
    })
  const remove = (m: Mailbox) =>
    gate.guard(mailStepUpScope.remove(m.address), async () => {
      await deleteMailbox(m.address)
      setDeleting(null)
      setOpen(null)
      void invalidate()
    })

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    const local = name.trim()
    if (!local || !chosen) return
    setBusy(true)
    setError(null)
    const create = () =>
      gate.guard({ action: 'manage', app: 'mail-mailbox' }, async () => {
        const res = await createMailbox(`${local}@${chosen}`, linked)
        setSecret({ address: res.mailbox.address, password: res.password })
        setName('')
        onAdding(false)
        void invalidate()
      })
    // The install domain is claimed for the default team the first time it is needed.
    if (needsClaim && isRoot) {
      await gate.guard(mailStepUpScope.domain(chosen), async () => {
        await claimMailDomain(chosen, teamId)
        await create()
      })
    } else {
      await create()
    }
    setBusy(false)
  }

  const chip = (m: Mailbox) =>
    m.suspend_pending ? (
      <Badge variant="warning">{t('mailboxes.status.pending')}</Badge>
    ) : m.suspended ? (
      <Badge variant="warning">{t('mailboxes.status.suspended')}</Badge>
    ) : (
      <Badge variant="success">{t('mailboxes.status.active')}</Badge>
    )

  return (
    <section aria-label={t('mailboxes.title')} className="space-y-3">
      {secret ? (
        <MailSecretReveal
          address={secret.address}
          password={secret.password}
          onDismiss={() => setSecret(null)}
        />
      ) : null}
      {error ? (
        <p role="alert" className="text-xs text-tone-danger">
          {error}
        </p>
      ) : null}
      {adding ? (
        <form
          onSubmit={(e) => void submit(e)}
          aria-label={t('shell.addMailbox')}
          className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card p-2"
        >
          <Input
            className="h-8 w-40"
            aria-label={t('mbx.add.name')}
            value={name}
            autoFocus
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') onAdding(false)
            }}
            placeholder={t('mbx.add.hint')}
          />
          {choices.length > 1 ? (
            <select
              aria-label={t('mbx.add.domainLabel')}
              className={`${FIELD_CLASS} min-w-40`}
              value={chosen}
              onChange={(e) => setDomain(e.target.value)}
            >
              {choices.map((d) => (
                <option key={d} value={d}>
                  @{d}
                </option>
              ))}
            </select>
          ) : (
            <Badge
              variant="muted"
              title={
                needsClaim ? t('mbx.add.claims', { domain: chosen }) : undefined
              }
            >
              @{chosen}
            </Badge>
          )}
          {(users?.length ?? 0) > 0 ? (
            <select
              aria-label={t('mbx.add.user')}
              className={`${FIELD_CLASS} w-40`}
              value={linked}
              onChange={(e) => setLinked(e.target.value)}
            >
              <option value="">{t('mailboxes.form.noUser')}</option>
              {users?.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.email}
                </option>
              ))}
            </select>
          ) : null}
          <span className="ml-auto flex items-center gap-1" title={writeWhy}>
            <Button
              type="submit"
              size="sm"
              disabled={!canWrite || busy || !name.trim() || !chosen}
            >
              {t('mbx.add.submit')}
            </Button>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              aria-label={t('mbx.add.close')}
              onClick={() => onAdding(false)}
            >
              <XGlassIcon size="1.1em" />
            </Button>
          </span>
        </form>
      ) : null}
      {boxesQ.isLoading ? (
        <div aria-label={t('mbx.loading')} className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : boxes.length === 0 ? (
        <div className="flex flex-wrap items-center gap-3 rounded-lg border border-dashed border-border px-3 py-3 text-sm">
          <InboxGlassIcon size="1.4em" className="text-muted-foreground" />
          <span className="text-muted-foreground">{t('mbx.empty')}</span>
          {!adding ? (
            <Button
              size="sm"
              className="ml-auto gap-1.5"
              onClick={() => onAdding(true)}
            >
              <PlusGlassIcon size="1.1em" />
              {t('shell.addMailbox')}
            </Button>
          ) : null}
        </div>
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative">
              <SearchGlassIcon
                size="1.1em"
                className="pointer-events-none absolute top-2 left-2 text-muted-foreground"
              />
              <Input
                type="search"
                className="h-8 w-56 pl-7"
                aria-label={t('mbx.search')}
                placeholder={t('mbx.search')}
                value={q}
                onChange={(e) => setQ(e.target.value)}
              />
            </div>
            <select
              aria-label={t('mbx.filter')}
              className={FIELD_CLASS}
              value={filter}
              onChange={(e) => setFilter(e.target.value as Filter)}
            >
              <option value="all">{t('mbx.all')}</option>
              <option value="active">{t('mbx.active')}</option>
              <option value="suspended">{t('mbx.suspended')}</option>
            </select>
          </div>
          {shown.length === 0 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">
              {t('mbx.noMatch')}
            </p>
          ) : (
            <ul
              aria-label={t('mbx.list')}
              className="divide-y divide-border rounded-lg border border-border bg-card"
            >
              {shown.map((m) => (
                <li
                  key={m.address}
                  className="group flex items-center gap-2 px-2 py-1.5 hover:bg-muted/50"
                >
                  <button
                    type="button"
                    onClick={() => setOpen(m.address)}
                    aria-label={t('mbx.details') + ': ' + m.address}
                    className="flex min-w-0 flex-1 items-center gap-3 rounded-md px-1 py-1 text-left focus-visible:outline-2 focus-visible:outline-ring"
                  >
                    <span
                      aria-hidden="true"
                      className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary uppercase"
                    >
                      {m.localpart.slice(0, 1)}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-mono text-sm">
                        {m.address}
                      </span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {teamName(m.team_id)}
                        {m.linked_user_id
                          ? ` · ${userName(m.linked_user_id)}`
                          : ''}
                      </span>
                    </span>
                    {sysFrom === m.address ? (
                      <Badge variant="muted">{t('mbx.system')}</Badge>
                    ) : null}
                    {chip(m)}
                  </button>
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <Button
                          size="icon-sm"
                          variant="ghost"
                          aria-label={t('mbx.actions', { address: m.address })}
                        />
                      }
                    >
                      <EllipsisGlassIcon size="1.2em" />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => setOpen(m.address)}>
                        {t('mbx.details')}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        disabled={!canWrite}
                        onClick={() => void reset(m)}
                      >
                        {t('mailboxes.reset')}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        disabled={!canWrite}
                        onClick={() => void toggle(m)}
                      >
                        {m.suspended
                          ? t('mailboxes.resume')
                          : t('mailboxes.suspend')}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        disabled={!isRoot}
                        onClick={() => setDeleting(m)}
                      >
                        {t('mailboxes.delete')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
      <MailMailboxSheet
        box={current}
        app={status?.app}
        teamName={current ? teamName(current.team_id) : ''}
        userName={current ? userName(current.linked_user_id) : ''}
        onClose={() => setOpen(null)}
        onToggle={(m) => void toggle(m)}
        onReset={(m) => void reset(m)}
        onDelete={setDeleting}
      />
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(o) => {
          if (!o) setDeleting(null)
        }}
        title={t('mailboxes.deleteTitle', { address: deleting?.address ?? '' })}
        description={t('mailboxes.deleteBody')}
        confirmLabel={t('mailboxes.delete')}
        tone="destructive"
        requireTyped={deleting?.address}
        focusCancel
        onConfirm={() => {
          if (deleting) void remove(deleting)
        }}
      />
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
    </section>
  )
}
