import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  InboxGlassIcon,
  KeyGlassIcon,
  PauseGlassIcon,
  PlayGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { useCanWriteSensitive } from '../../hooks/useCanWriteSensitive'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  deleteMailbox,
  mailStepUpScope,
  resetMailboxPassword,
  setMailboxSuspended,
  useInvalidateMail,
  useMailboxes,
  type MailDomain,
  type Mailbox,
} from '../../queries/mailMailboxes'
import { teamListQueryOptions } from '../../queries/teams'
import { userListQueryOptions } from '../../queries/users'
import { StepUpDialog } from '../env/StepUpDialog'
import { MailAddMailboxForm } from './MailAddMailboxForm'
import { MailDomainClaim } from './MailDomainClaim'
import { MailSecretReveal } from './MailSecretReveal'
import { PanelCard, Reason } from './mailPanelUi'

export function MailMailboxesPanel({ domains }: { domains: MailDomain[] }) {
  const { t } = useTranslation('mailServer')
  const boxes = useMailboxes().data ?? []
  const teams = useQuery(teamListQueryOptions()).data
  const users = useQuery(userListQueryOptions()).data
  const canWrite = useCanWriteSensitive()
  const isRoot = useIsRoot()
  const invalidate = useInvalidateMail()
  const [secret, setSecret] = useState<{
    address: string
    password: string
  } | null>(null)
  const [deleting, setDeleting] = useState<Mailbox | null>(null)
  const [error, setError] = useState<string | null>(null)
  const onError = useCallback(
    (e: unknown) =>
      setError(e instanceof Error ? e.message : t('mailboxes.failed')),
    [t],
  )
  const gate = useStepUpGate(onError)
  const teamName = (id: string) => teams?.find((x) => x.id === id)?.name ?? id
  const userName = (id?: string) =>
    id ? (users?.find((u) => u.id === id)?.email ?? id) : t('mailboxes.noUser')

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
      void invalidate()
    })

  const writeWhy = canWrite ? undefined : t('mailboxes.needsWrite')
  return (
    <PanelCard label={t('mailboxes.title')} title={t('mailboxes.title')}>
      <MailDomainClaim domains={domains} isRoot={isRoot} teams={teams ?? []} />
      {domains.length > 0 && (
        <MailAddMailboxForm
          domains={domains}
          users={users ?? []}
          canWrite={canWrite}
          onSecret={(address, password) => setSecret({ address, password })}
        />
      )}
      {secret && (
        <MailSecretReveal
          address={secret.address}
          password={secret.password}
          onDismiss={() => setSecret(null)}
        />
      )}
      {error && (
        <p role="alert" className="text-xs text-tone-danger">
          {error}
        </p>
      )}
      {boxes.length === 0 ? (
        <EmptyState
          icon={<InboxGlassIcon size="1.6em" />}
          title={t('mailboxes.empty.title')}
          description={
            domains.length === 0
              ? t('mailboxes.empty.claim')
              : t('mailboxes.empty.add')
          }
        />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="text-muted-foreground">
              <tr>
                <th className="py-1 pr-3 font-medium">
                  {t('mailboxes.col.address')}
                </th>
                <th className="py-1 pr-3 font-medium">
                  {t('mailboxes.col.team')}
                </th>
                <th className="py-1 pr-3 font-medium">
                  {t('mailboxes.col.user')}
                </th>
                <th className="py-1 pr-3 font-medium">
                  {t('mailboxes.col.status')}
                </th>
                <th className="py-1 pr-3 font-medium">
                  {t('mailboxes.col.created')}
                </th>
                <th className="py-1 font-medium">
                  <span className="sr-only">{t('mailboxes.col.actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {boxes.map((m) => (
                <tr
                  key={m.address}
                  className="border-t border-border align-top"
                >
                  <td className="py-1.5 pr-3 font-mono">{m.address}</td>
                  <td className="py-1.5 pr-3">{teamName(m.team_id)}</td>
                  <td className="py-1.5 pr-3">{userName(m.linked_user_id)}</td>
                  <td className="py-1.5 pr-3">
                    {m.suspend_pending ? (
                      <span className="text-tone-warning">
                        {t('mailboxes.status.pending')}
                        <span className="block text-muted-foreground">
                          {t('mailboxes.pendingWarning')}
                        </span>
                      </span>
                    ) : m.suspended ? (
                      <span className="text-tone-warning">
                        {t('mailboxes.status.suspended')}
                      </span>
                    ) : (
                      t('mailboxes.status.active')
                    )}
                  </td>
                  <td className="py-1.5 pr-3 whitespace-nowrap">
                    {new Date(m.created_at).toLocaleDateString()}
                  </td>
                  <td className="py-1.5">
                    <div className="flex flex-wrap justify-end gap-1">
                      <Reason why={writeWhy}>
                        <Button
                          size="sm"
                          variant="outline"
                          className="gap-1"
                          disabled={!canWrite}
                          aria-label={t(
                            m.suspended
                              ? 'mailboxes.resumeFor'
                              : 'mailboxes.suspendFor',
                            { address: m.address },
                          )}
                          onClick={() => void toggle(m)}
                        >
                          {m.suspended ? (
                            <PlayGlassIcon size="1.1em" />
                          ) : (
                            <PauseGlassIcon size="1.1em" />
                          )}
                          {m.suspended
                            ? t('mailboxes.resume')
                            : t('mailboxes.suspend')}
                        </Button>
                      </Reason>
                      <Reason why={writeWhy}>
                        <Button
                          size="sm"
                          variant="outline"
                          className="gap-1"
                          disabled={!canWrite}
                          aria-label={t('mailboxes.resetFor', {
                            address: m.address,
                          })}
                          onClick={() => void reset(m)}
                        >
                          <KeyGlassIcon size="1.1em" />
                          {t('mailboxes.reset')}
                        </Button>
                      </Reason>
                      <Reason
                        why={isRoot ? undefined : t('mailboxes.needsRoot')}
                      >
                        <Button
                          size="sm"
                          variant="outline"
                          className="gap-1"
                          disabled={!isRoot}
                          aria-label={t('mailboxes.deleteFor', {
                            address: m.address,
                          })}
                          onClick={() => setDeleting(m)}
                        >
                          <TrashGlassIcon size="1.1em" />
                          {t('mailboxes.delete')}
                        </Button>
                      </Reason>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
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
    </PanelCard>
  )
}
