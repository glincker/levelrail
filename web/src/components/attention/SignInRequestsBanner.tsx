import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { KeyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { SignInRequestCard } from '../signin/SignInRequestCard'
import {
  revealLoginCode,
  signInRequestsQueryOptions,
  type SignInApprovalRequest,
  type SignInCodeRequest,
} from '../../queries/signIn'

// Shown on every page while someone is signing in to this account with a
// code or from a new browser. It never fetches a code until asked.
export function SignInRequestsBanner() {
  const { data } = useQuery(signInRequestsQueryOptions())
  const { t } = useTranslation('signIn', { useSuspense: false })
  const codes = data?.codes ?? []
  const approvals = data?.approvals ?? []
  if (codes.length === 0 && approvals.length === 0) {
    return null
  }
  return (
    <div
      role="alert"
      className="shrink-0 space-y-2 border-b border-primary/30 bg-primary/10 px-4 py-2 text-sm"
    >
      <p className="font-medium">{t('requests.bannerTitle')}</p>
      <SignInRequestsList codes={codes} approvals={approvals} compact />
    </div>
  )
}

export function SignInRequestsList({
  codes,
  approvals,
  compact = false,
}: {
  codes: SignInCodeRequest[]
  approvals: SignInApprovalRequest[]
  compact?: boolean
}) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  // Decided requests leave the server list; keep them so the result stays visible.
  const [seen, setSeen] = useState<Record<string, SignInApprovalRequest>>({})
  if (approvals.some((a) => seen[a.id] === undefined)) {
    setSeen((s) => ({
      ...s,
      ...Object.fromEntries(approvals.map((a) => [a.id, a])),
    }))
  }
  const live = new Set(approvals.map((a) => a.id))
  const shown = [
    ...approvals,
    ...Object.values(seen).filter((a) => !live.has(a.id)),
  ]
  if (codes.length === 0 && shown.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('requests.empty')}</p>
    )
  }
  return (
    <ul className="space-y-3">
      {codes.map((c) => (
        <CodeRow key={c.id} request={c} />
      ))}
      {shown.map((a) => (
        <li key={a.id}>
          <SignInRequestCard
            request={a}
            compact={compact}
            gone={!live.has(a.id)}
          />
        </li>
      ))}
    </ul>
  )
}

function RequestContext({
  ip,
  agent,
  at,
}: {
  ip: string
  agent: string
  at: string
}) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  return (
    <>
      <p className="text-xs text-muted-foreground">
        {t('requests.context', {
          ip,
          agent,
          time: new Date(at).toLocaleString(),
        })}
      </p>
      <p className="text-xs text-muted-foreground">{t('requests.warning')}</p>
    </>
  )
}

function CodeRow({ request }: { request: SignInCodeRequest }) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const [code, setCode] = useState<string | null>(null)
  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2">
      <KeyIcon aria-hidden="true" className="size-5 shrink-0" />
      <div className="min-w-0 flex-1">
        <p className="font-medium">{t('requests.codeTitle')}</p>
        <RequestContext
          ip={request.requester_ip}
          agent={request.user_agent}
          at={request.created_at}
        />
      </div>
      {code ? (
        <span
          className="font-mono text-xl font-semibold tracking-widest"
          data-testid="sign-in-code"
        >
          {code}
        </span>
      ) : null}
      {request.revealable ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            if (code) {
              setCode(null)
              return
            }
            revealLoginCode(request.id)
              .then((res) => setCode(res.code))
              .catch((err: unknown) => {
                toast.add({
                  title:
                    err instanceof Error
                      ? err.message
                      : t('requests.actionFailed'),
                  type: 'error',
                })
              })
          }}
        >
          {code ? t('requests.hide') : t('requests.show')}
        </Button>
      ) : (
        <p className="text-xs text-muted-foreground">
          {t('requests.notRevealable')}
        </p>
      )}
    </li>
  )
}
