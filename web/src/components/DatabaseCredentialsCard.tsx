import { useState } from 'react'
import {
  CheckIcon,
  CopyIcon,
  EyeIcon,
  EyeSlashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useDatabaseCredentials } from '../queries/databases'

// One row: a label, a value (masked or plain), and a copy button. Used
// for every field below rather than one big connection-string blob, so
// an operator who only needs the password (say, to paste into another
// tool's own host/port fields) doesn't have to parse a URL apart by
// hand.
function CredentialRow({
  label,
  value,
  mask,
}: {
  label: string
  value: string
  mask: boolean
}) {
  const [copied, setCopied] = useState(false)

  function copy() {
    void navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
    })
  }

  return (
    <div className="flex items-center gap-2">
      <span className="w-20 shrink-0 text-sm font-medium text-muted-foreground">
        {label}
      </span>
      <code className="min-w-0 flex-1 overflow-x-auto rounded-md border border-input bg-muted/50 px-2 py-1 text-xs break-all">
        {mask ? '•'.repeat(Math.min(value.length, 24)) : value}
      </code>
      <Button type="button" size="sm" variant="outline" onClick={copy}>
        {copied ? <CheckIcon /> : <CopyIcon />}
        {copied ? 'Copied' : 'Copy'}
      </Button>
    </div>
  )
}

// DatabaseCredentialsCard is the dashboard counterpart to GET
// /api/v1/databases/{name}/credentials (internal/api/
// database_credentials.go): host/port/username/password/url for
// plugging this database into an external client (TablePlus, DBeaver,
// redis-cli run from an operator's own machine), without shelling into
// the container first via the Exec tab. Fetched only on an explicit
// "Reveal" click, never on page load, since the response carries a real
// secret's plaintext; the password itself stays masked behind a second,
// separate toggle even after revealing the rest, so a screen share or a
// screenshot of this card doesn't leak it by accident.
export function DatabaseCredentialsCard({ name }: { name: string }) {
  const credentials = useDatabaseCredentials()
  const [passwordVisible, setPasswordVisible] = useState(false)

  const data = credentials.data

  return (
    <Card>
      <CardHeader>
        <CardTitle>Connection credentials</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {!data ? (
          <Button
            type="button"
            variant="outline"
            disabled={credentials.isPending}
            onClick={() => {
              credentials.mutate(name)
            }}
          >
            {credentials.isPending ? 'Loading...' : 'Reveal credentials'}
          </Button>
        ) : (
          <div className="space-y-2">
            <CredentialRow label="Host" value={data.host} mask={false} />
            <CredentialRow label="Port" value={String(data.port)} mask={false} />
            {data.database ? (
              <CredentialRow label="Database" value={data.database} mask={false} />
            ) : null}
            {data.username ? (
              <CredentialRow label="Username" value={data.username} mask={false} />
            ) : null}
            {data.password ? (
              <div className="flex items-center gap-2">
                <span className="w-20 shrink-0 text-sm font-medium text-muted-foreground">
                  Password
                </span>
                <code className="min-w-0 flex-1 overflow-x-auto rounded-md border border-input bg-muted/50 px-2 py-1 text-xs break-all">
                  {passwordVisible
                    ? data.password
                    : '•'.repeat(Math.min(data.password.length, 24))}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setPasswordVisible((v) => !v)
                  }}
                  aria-label={passwordVisible ? 'Hide password' : 'Show password'}
                >
                  {passwordVisible ? <EyeSlashIcon /> : <EyeIcon />}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    void navigator.clipboard.writeText(data.password ?? '')
                  }}
                >
                  <CopyIcon />
                  Copy
                </Button>
              </div>
            ) : null}
            <CredentialRow label="URL" value={data.url} mask={!passwordVisible && Boolean(data.password)} />
          </div>
        )}

        {credentials.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{credentials.error.message}</AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  )
}
