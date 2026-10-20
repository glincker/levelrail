import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BuildingsIcon } from '@phosphor-icons/react/dist/ssr'
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

  const submit = async () => {
    setBusy(true)
    setMissed(false)
    try {
      const domain = emailDomain(email)
      const route = domain ? await fetchSSORoute(domain) : null
      if (route) {
        window.location.assign(route.start_url)
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
          <BuildingsIcon aria-hidden="true" />
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
          onChange={(e) => setEmail(e.target.value)}
        />
        <Button type="submit" disabled={busy || email.trim() === ''}>
          {t('foundation.signIn.ssoContinue')}
        </Button>
      </div>
      {missed ? (
        <p role="status" className="text-xs text-muted-foreground">
          {t('foundation.signIn.ssoNoRoute')}
        </p>
      ) : null}
    </form>
  )
}
