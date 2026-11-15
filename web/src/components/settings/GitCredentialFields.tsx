import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { SecretInput } from '@/components/ui/secret-input'
import { gitCredentialIssue, type GitConnectProvider } from '../../lib/gitHost'

// Client id and secret inputs with inline checks after the field is left.
// Only shape and emptiness are checked, and the value is never repeated.
export function GitCredentialFields({
  provider,
  clientId,
  clientSecret,
  onClientId,
  onClientSecret,
}: Readonly<{
  provider: GitConnectProvider
  clientId: string
  clientSecret: string
  onClientId: (value: string) => void
  onClientSecret: (value: string) => void
}>) {
  const { t } = useTranslation('settings')
  const { t: tc } = useTranslation('gitConnect')
  const [seen, setSeen] = useState({ id: false, secret: false })
  const idIssue = seen.id ? gitCredentialIssue(provider, 'id', clientId) : null
  const secretIssue = seen.secret
    ? gitCredentialIssue(provider, 'secret', clientSecret)
    : null
  const name = tc(`name.${provider}`)

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <Field>
        <FieldLabel htmlFor="git-conn-client-id">
          {t(`gitConnections.add.clientId.${provider}`)}
        </FieldLabel>
        <Input
          id="git-conn-client-id"
          autoComplete="off"
          spellCheck={false}
          className="font-mono"
          value={clientId}
          aria-invalid={idIssue === 'empty' || idIssue === 'whitespace'}
          aria-describedby={idIssue ? 'git-conn-client-id-msg' : undefined}
          onChange={(e) => onClientId(e.target.value)}
          onBlur={() => setSeen((s) => ({ ...s, id: true }))}
        />
        <IssueText id="git-conn-client-id-msg" issue={idIssue} name={name} />
      </Field>
      <Field>
        <FieldLabel htmlFor="git-conn-client-secret">
          {t('gitConnections.add.clientSecret')}
        </FieldLabel>
        <SecretInput
          id="git-conn-client-secret"
          autoComplete="off"
          spellCheck={false}
          value={clientSecret}
          aria-invalid={secretIssue === 'empty' || secretIssue === 'whitespace'}
          aria-describedby={
            secretIssue ? 'git-conn-client-secret-msg' : undefined
          }
          onChange={(e) => onClientSecret(e.target.value)}
          onBlur={() => setSeen((s) => ({ ...s, secret: true }))}
        />
        <IssueText
          id="git-conn-client-secret-msg"
          issue={secretIssue}
          name={name}
        />
      </Field>
    </div>
  )
}

function IssueText({
  id,
  issue,
  name,
}: Readonly<{
  id: string
  issue: ReturnType<typeof gitCredentialIssue>
  name: string
}>) {
  const { t } = useTranslation('gitConnect')
  if (!issue) return null
  return (
    <p
      id={id}
      role={issue === 'unusual' ? 'status' : 'alert'}
      className={
        issue === 'unusual'
          ? 'text-xs text-muted-foreground'
          : 'text-xs text-destructive'
      }
    >
      {t(`field.${issue}`, { provider: name })}
    </p>
  )
}
