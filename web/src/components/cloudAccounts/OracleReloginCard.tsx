import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { ociSessionCommand } from '../../lib/ociSession'
import { formatRelativeTime } from '../../lib/relativeTime'
import { useReloginOracleSession } from '../../queries/cloudAccounts'
import type { CloudAccount } from '../../types/cloudAccounts'

// Shown for Oracle browser session accounts: expiry as relative time, and a relogin form when needed.
export function OracleReloginCard({
  account,
}: Readonly<{ account: CloudAccount }>) {
  const { t } = useTranslation('cloudAccounts')
  const uid = useId()
  const [token, setToken] = useState('')
  const [key, setKey] = useState('')
  const relogin = useReloginOracleSession(account.id)
  if (account.method !== 'oci_session') return null

  function submit(e: React.FormEvent) {
    e.preventDefault()
    relogin.mutate(
      { session_token: token.trim(), private_key_pem: key },
      {
        onSuccess: () => {
          setToken('')
          setKey('')
          toast.add({ title: t('relogin.done'), type: 'success' })
        },
      },
    )
  }

  return (
    <section
      aria-label={t('relogin.title')}
      className="space-y-3 rounded-lg border border-border p-3 text-sm"
    >
      <h3 className="font-medium">{t('relogin.title')}</h3>
      {account.session_expires_at ? (
        <p className="text-muted-foreground">
          {t(account.needs_relogin ? 'relogin.expired' : 'relogin.expires', {
            when: formatRelativeTime(account.session_expires_at),
          })}
        </p>
      ) : null}
      {account.needs_relogin ? (
        <form onSubmit={submit} className="space-y-3" autoComplete="off">
          <code className="block overflow-x-auto rounded-md border border-border bg-muted/40 p-2 font-mono text-xs">
            {ociSessionCommand(account.config.region ?? '')}
          </code>
          <Field>
            <FieldLabel htmlFor={`${uid}-token`}>
              {t('relogin.token')}
            </FieldLabel>
            <Textarea
              id={`${uid}-token`}
              value={token}
              onChange={(e) => setToken(e.target.value)}
              rows={3}
              spellCheck={false}
              className="font-mono text-xs"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${uid}-key`}>{t('relogin.key')}</FieldLabel>
            <Textarea
              id={`${uid}-key`}
              value={key}
              onChange={(e) => setKey(e.target.value)}
              rows={4}
              spellCheck={false}
              placeholder="-----BEGIN PRIVATE KEY-----"
              className="font-mono text-xs"
            />
          </Field>
          {relogin.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertDescription>{relogin.error.message}</AlertDescription>
            </Alert>
          ) : null}
          <Button
            type="submit"
            disabled={
              token.trim() === '' || key.trim() === '' || relogin.isPending
            }
          >
            {t('relogin.submit')}
          </Button>
        </form>
      ) : null}
    </section>
  )
}
