import { useEffect, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  useAwsSsoPoll,
  useAwsSsoRelogin,
  useAwsSsoSelect,
  useAwsSsoStart,
} from '../../queries/cloudAccounts'
import type { AwsSsoAccount, AwsSsoStart } from '../../types/cloudAccounts'

interface Props {
  // Set for a relogin of an existing account; empty starts a new link.
  reloginAccountId?: string
  onDone?: () => void
}

const FIELD =
  'w-full rounded-md border border-input bg-background px-3 py-2 text-sm'

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function AwsSsoLogin({ reloginAccountId, onDone }: Props) {
  const { t } = useTranslation('cloudAccounts')
  const navigate = useNavigate()
  const [startUrl, setStartUrl] = useState('')
  const [region, setRegion] = useState('')
  const [session, setSession] = useState<AwsSsoStart | null>(null)
  const [accounts, setAccounts] = useState<AwsSsoAccount[] | null>(null)
  const [pick, setPick] = useState<{ account: string; role: string } | null>(
    null,
  )
  const [label, setLabel] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [finished, setFinished] = useState(false)
  const start = useAwsSsoStart()
  const relogin = useAwsSsoRelogin()
  const poll = useAwsSsoPoll()
  const select = useAwsSsoSelect()
  const timer = useRef<number | null>(null)
  const delay = useRef(5)

  useEffect(
    () => () => {
      if (timer.current !== null) window.clearTimeout(timer.current)
    },
    [],
  )

  function schedule(sessionId: string) {
    timer.current = window.setTimeout(() => {
      poll.mutate(sessionId, {
        onSuccess: (r) => {
          if (r.status === 'done') {
            if (r.account_id) {
              setFinished(true)
              onDone?.()
            } else setAccounts(r.accounts ?? [])
            return
          }
          if (r.status === 'denied') {
            setSession(null)
            setError(t('sso.denied'))
            return
          }
          if (r.interval) delay.current = r.interval
          schedule(sessionId)
        },
        onError: (e) => {
          setSession(null)
          setError(messageOf(e))
        },
      })
    }, delay.current * 1000)
  }

  function begun(s: AwsSsoStart) {
    setSession(s)
    delay.current = s.interval
    schedule(s.session_id)
  }

  function go() {
    setError(null)
    const opts = {
      onSuccess: begun,
      onError: (e: unknown) => {
        setError(messageOf(e))
      },
    }
    if (reloginAccountId) relogin.mutate(reloginAccountId, opts)
    else
      start.mutate({ start_url: startUrl.trim(), region: region.trim() }, opts)
  }

  function link() {
    if (!session || !pick) return
    select.mutate(
      {
        session_id: session.session_id,
        account_id: pick.account,
        role_name: pick.role,
        name: label.trim() || undefined,
      },
      {
        onSuccess: (a) => {
          void navigate({
            to: '/integrations/cloud-accounts/$id',
            params: { id: a.id },
          })
        },
        onError: (e) => {
          setError(messageOf(e))
        },
      },
    )
  }

  if (finished) {
    return (
      <p role="status" className="text-sm">
        {t('sso.reloginDone')}
      </p>
    )
  }

  return (
    <div className="space-y-3">
      {!session ? (
        <div className="space-y-3">
          {reloginAccountId ? null : (
            <>
              <label className="block space-y-1 text-sm">
                <span>{t('sso.startUrl')}</span>
                <input
                  className={FIELD}
                  value={startUrl}
                  placeholder={t('sso.startUrlExample')}
                  onChange={(e) => {
                    setStartUrl(e.target.value)
                  }}
                />
              </label>
              <label className="block space-y-1 text-sm">
                <span>{t('sso.region')}</span>
                <input
                  className={FIELD}
                  value={region}
                  placeholder={t('sso.regionExample')}
                  onChange={(e) => {
                    setRegion(e.target.value)
                  }}
                />
              </label>
            </>
          )}
          <Button
            size="sm"
            disabled={
              start.isPending ||
              relogin.isPending ||
              (!reloginAccountId && (!startUrl.trim() || !region.trim()))
            }
            onClick={go}
          >
            {t('sso.start')}
          </Button>
        </div>
      ) : null}
      {session && !accounts ? (
        <div className="space-y-2" role="status">
          <p className="text-sm">{t('sso.enterCode')}</p>
          <p className="font-mono text-2xl tracking-widest">
            {session.user_code}
          </p>
          <a
            className="text-sm underline"
            href={session.verification_uri_complete}
            target="_blank"
            rel="noreferrer"
          >
            {t('sso.openBrowser')}
          </a>
          <p className="text-sm text-muted-foreground">{t('sso.waiting')}</p>
        </div>
      ) : null}
      {accounts ? (
        <div className="space-y-3">
          <p className="text-sm">{t('sso.pickRole')}</p>
          {accounts.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('sso.noAccounts')}
            </p>
          ) : null}
          <ul className="space-y-2">
            {accounts.map((a) => (
              <li key={a.account_id} className="rounded-md border p-2 text-sm">
                <p className="font-medium">
                  {a.account_name}{' '}
                  <span className="font-mono text-muted-foreground">
                    {a.account_id}
                  </span>
                </p>
                <div className="mt-1 flex flex-wrap gap-2">
                  {a.roles.map((r) => (
                    <Button
                      key={r}
                      size="sm"
                      variant={
                        pick?.account === a.account_id && pick.role === r
                          ? 'default'
                          : 'outline'
                      }
                      onClick={() => {
                        setPick({ account: a.account_id, role: r })
                      }}
                    >
                      {r}
                    </Button>
                  ))}
                </div>
              </li>
            ))}
          </ul>
          <label className="block space-y-1 text-sm">
            <span>{t('sso.label')}</span>
            <input
              className={FIELD}
              value={label}
              onChange={(e) => {
                setLabel(e.target.value)
              }}
            />
          </label>
          <Button size="sm" disabled={!pick || select.isPending} onClick={link}>
            {t('sso.link')}
          </Button>
        </div>
      ) : null}
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
    </div>
  )
}
