import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { UniversityGlassIcon } from '@thesvg/ui-react'
import type { SSORoute } from '../queries/ssoOidc'
import { emailDomain, fetchSSORoute } from '../queries/ssoOidc'
import { Button } from './ui/button'
import { Input } from './ui/input'

/**
 * SSOEmailRoute sends a typed work email to the connection that claims its
 * domain. Only the domain leaves the browser, and every miss reads the same.
 */
export function SSOEmailRoute() {
  const { t } = useTranslation('common')
  const [open, setOpen] = useState(false)
  const [email, setEmail] = useState('')
  const [busy, setBusy] = useState(false)
  const [missed, setMissed] = useState(false)
  const [route, setRoute] = useState<SSORoute | null>(null)

  const submit = async () => {
    setBusy(true)
    setMissed(false)
    setRoute(null)
    try {
      const domain = emailDomain(email)
      const found = domain ? await fetchSSORoute(domain) : null
      if (found) {
        setRoute(found)
        return
      }
      setMissed(true)
    } catch {
      setMissed(true)
    } finally {
      setBusy(false)
    }
  }

  if (!open) {
    return (
      <Button
        type="button"
        variant="outline"
        size="lg"
        className="justify-start gap-3"
        onClick={() => setOpen(true)}
      >
        <span className="flex size-6 items-center justify-center rounded bg-muted text-muted-foreground">
          <UniversityGlassIcon size="1.2em" aria-hidden="true" />
        </span>
        {t('foundation.signIn.ssoContinue')}
      </Button>
    )
  }
  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <label htmlFor="sso-email" className="text-xs text-muted-foreground">
        {t('foundation.signIn.ssoEmail')}
      </label>
      <div className="flex gap-2">
        <Input
          id="sso-email"
          type="email"
          autoComplete="email"
          autoFocus
          value={email}
          onChange={(e) => {
            setEmail(e.target.value)
            setRoute(null)
          }}
        />
        <Button type="submit" disabled={busy || email.trim() === ''}>
          {t('foundation.signIn.ssoCheck')}
        </Button>
      </div>
      {route ? (
        <Button
          type="button"
          size="lg"
          onClick={() => window.location.assign(route.start_url)}
        >
          {t('foundation.signIn.ssoContinueWith', { name: route.name })}
        </Button>
      ) : null}
      {missed ? (
        <p role="status" className="text-xs text-muted-foreground">
          {t('foundation.signIn.ssoNoRoute')}
        </p>
      ) : null}
    </form>
  )
}
