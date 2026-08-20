import { useState } from 'react'
import { LockIcon, LockKeyIcon, LockOpenIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  useClearDomainBasicAuth,
  useDomainBasicAuth,
  useSetDomainBasicAuth,
} from '../queries/domainBasicAuth'

// HTTP Basic Auth protection for one already-saved domain, enforced by
// the embedded Caddy ingress on the next reconcile pass
// (internal/reconcile/ingress). Collapsed by default (a badge plus a
// toggle) so a list of several domains, either here or in
// AppNetworkPanel, doesn't turn into a wall of open forms; expands into
// the same username/password/clear form CloudflareTunnelCard already
// establishes for a comparable has_password-gated credential.
export function DomainBasicAuthControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { data: auth, isLoading } = useDomainBasicAuth(appName, domain)
  const [open, setOpen] = useState(false)
  // usernameEdit/usernameTouched track a local override once the
  // operator starts typing, without syncing auth?.username into state
  // via an effect (that pattern cascades an extra render on every fetch
  // and React's own docs steer away from it): until touched, the field
  // just displays the fetched value directly.
  const [usernameEdit, setUsernameEdit] = useState('')
  const [usernameTouched, setUsernameTouched] = useState(false)
  const username = usernameTouched ? usernameEdit : (auth?.username ?? '')
  const [password, setPassword] = useState('')
  const [formError, setFormError] = useState<string | null>(null)
  const setAuth = useSetDomainBasicAuth(appName, domain)
  const clearAuth = useClearDomainBasicAuth(appName, domain)

  if (isLoading) return null

  const protectedNow = auth?.enabled ?? false
  const pending = setAuth.isPending || clearAuth.isPending

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (!username.trim()) {
      setFormError('A username is required.')
      return
    }
    if (!auth?.has_password && !password.trim()) {
      setFormError('A password is required the first time this domain is protected.')
      return
    }
    setAuth.mutate(
      { username: username.trim(), password: password.trim() || undefined },
      { onSuccess: () => setPassword('') },
    )
  }

  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className="flex items-center gap-1.5 text-left"
        >
          {protectedNow ? (
            <Badge variant="success" className="shrink-0">
              <LockKeyIcon className="size-3" />
              Basic auth on
            </Badge>
          ) : (
            <Badge variant="muted" className="shrink-0">
              <LockOpenIcon className="size-3" />
              Not protected
            </Badge>
          )}
        </button>
        <Button type="button" variant="ghost" size="sm" onClick={() => setOpen((v) => !v)}>
          <LockIcon className="size-3.5" />
          {open ? 'Hide' : protectedNow ? 'Manage' : 'Protect with basic auth'}
        </Button>
      </div>

      {open ? (
        <form
          onSubmit={(e) => {
            handleSubmit(e)
          }}
          className="mt-3 space-y-3"
        >
          <Field>
            <FieldLabel htmlFor={`basic-auth-username-${domain}`}>Username</FieldLabel>
            <Input
              id={`basic-auth-username-${domain}`}
              autoComplete="off"
              value={username}
              onChange={(e) => {
                setUsernameEdit(e.target.value)
                setUsernameTouched(true)
              }}
              disabled={pending}
              placeholder="operator"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`basic-auth-password-${domain}`}>Password</FieldLabel>
            <Input
              id={`basic-auth-password-${domain}`}
              type="password"
              autoComplete="off"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={pending}
              placeholder={auth?.has_password ? '••••••••••••' : 'Choose a password'}
            />
            <FieldDescription>
              {auth?.has_password
                ? 'A password is already set. Leave blank to keep it.'
                : 'Anyone visiting this domain will be asked for these credentials.'}
            </FieldDescription>
          </Field>

          {formError ? (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          ) : null}
          {setAuth.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{setAuth.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {clearAuth.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{clearAuth.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={pending}>
              {setAuth.isPending ? 'Saving...' : 'Save'}
            </Button>
            {protectedNow ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={pending}
                onClick={() => {
                  clearAuth.mutate(undefined, { onSuccess: () => setPassword('') })
                }}
              >
                {clearAuth.isPending ? 'Removing...' : 'Remove protection'}
              </Button>
            ) : null}
          </div>
        </form>
      ) : null}
    </div>
  )
}
