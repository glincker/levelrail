import { useCallback, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  createMailbox,
  useInvalidateMail,
  type MailDomain,
} from '../../queries/mailMailboxes'
import type { UserResource } from '../../queries/users'
import { FIELD_CLASS, Reason } from './mailPanelUi'

interface Props {
  domains: MailDomain[]
  users: UserResource[]
  canWrite: boolean
  onSecret: (address: string, password: string) => void
}

export function MailAddMailboxForm({
  domains,
  users,
  canWrite,
  onSecret,
}: Props) {
  const { t } = useTranslation('mailServer')
  const invalidate = useInvalidateMail()
  const [domain, setDomain] = useState(domains[0]?.domain ?? '')
  const [local, setLocal] = useState('')
  const [linked, setLinked] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const onError = useCallback(
    (e: unknown) =>
      setError(e instanceof Error ? e.message : t('mailboxes.failed')),
    [t],
  )
  const gate = useStepUpGate(onError)
  const chosen = domain || domains[0]?.domain || ''

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!chosen || !local.trim()) return
    setBusy(true)
    setError(null)
    await gate.guard({ action: 'manage', app: 'mail-mailbox' }, async () => {
      const res = await createMailbox(`${local.trim()}@${chosen}`, linked)
      onSecret(res.mailbox.address, res.password)
      setLocal('')
      void invalidate()
    })
    setBusy(false)
  }

  return (
    <div className="space-y-2">
      <form
        onSubmit={(e) => void submit(e)}
        className="flex flex-wrap items-end gap-2"
      >
        <div className="flex flex-col gap-1">
          <Label htmlFor="mb-local" className="text-xs">
            {t('mailboxes.form.localpart')}
          </Label>
          <Input
            id="mb-local"
            className="h-8 w-36"
            value={local}
            onChange={(e) => setLocal(e.target.value)}
            placeholder={t('mailboxes.form.localpartHint')}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="mb-domain" className="text-xs">
            {t('mailboxes.form.domain')}
          </Label>
          <select
            id="mb-domain"
            className={FIELD_CLASS}
            value={chosen}
            onChange={(e) => setDomain(e.target.value)}
          >
            {domains.map((d) => (
              <option key={d.domain} value={d.domain}>
                @{d.domain}
              </option>
            ))}
          </select>
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="mb-user" className="text-xs">
            {t('mailboxes.form.linkedUser')}
          </Label>
          <select
            id="mb-user"
            className={FIELD_CLASS}
            value={linked}
            onChange={(e) => setLinked(e.target.value)}
          >
            <option value="">{t('mailboxes.form.noUser')}</option>
            {users.map((u) => (
              <option key={u.id} value={u.id}>
                {u.email}
              </option>
            ))}
          </select>
        </div>
        <Reason why={canWrite ? undefined : t('mailboxes.needsWrite')}>
          <Button
            type="submit"
            size="sm"
            className="gap-1"
            disabled={!canWrite || busy || !local.trim()}
          >
            <PlusGlassIcon size="1.1em" />
            {t('mailboxes.form.add')}
          </Button>
        </Reason>
      </form>
      {error && (
        <p role="alert" className="text-xs text-tone-danger">
          {error}
        </p>
      )}
    </div>
  )
}
