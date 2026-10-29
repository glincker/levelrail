import { useState, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  CopyGlassIcon,
  CountdownGlassIcon,
  InfoGlassIcon,
  ShieldAlertGlassIcon,
  ShieldCheckGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { ApiError } from '../../lib/apiError'
import { ipScope, isValidMatch, secondsLeft } from '../../lib/signInRequest'
import {
  useDecideApproval,
  type SignInApprovalRequest,
} from '../../queries/signIn'
import { MatchDigits } from './MatchDigits'
import { SignInRequestFacts } from './SignInRequestFacts'
import { deviceTitle, useNow } from './signInRequestHooks'

type Outcome = 'approved' | 'denied' | 'handled' | 'expired' | 'failed'

const GONE_STATUSES = new Set([404, 409, 410])

// Raised buttons that sink a pixel when pressed; no movement under reduced motion.
const PRESS =
  'transition-[transform,box-shadow] active:translate-y-px active:shadow-none motion-reduce:transition-none motion-reduce:active:translate-y-0'

function Result({
  tone,
  icon,
  title,
  children,
}: {
  tone: 'success' | 'neutral' | 'danger'
  icon: ReactNode
  title: string
  children?: ReactNode
}) {
  return (
    <div
      role="status"
      className={cn(
        'flex items-start gap-3 rounded-lg border p-3 text-sm',
        tone === 'success' &&
          'border-tone-success-border bg-tone-success-soft text-tone-success',
        tone === 'danger' &&
          'border-tone-danger-border bg-tone-danger-soft text-tone-danger',
        tone === 'neutral' && 'border-border bg-muted text-foreground',
      )}
    >
      <span aria-hidden="true" className="mt-0.5 shrink-0">
        {icon}
      </span>
      <div className="min-w-0 space-y-1">
        <p className="font-medium">{title}</p>
        {children}
      </div>
    </div>
  )
}

function RequestDetails({ id }: { id: string }) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const [open, setOpen] = useState(false)
  return (
    <div className="text-xs text-muted-foreground">
      <button
        type="button"
        aria-expanded={open}
        className="inline-flex items-center gap-1 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
        onClick={() => setOpen((v) => !v)}
      >
        <ChevronDownGlassIcon
          aria-hidden="true"
          className={cn('size-4.5 transition-transform', open && 'rotate-180')}
        />
        {t('request.details')}
      </button>
      {open ? (
        <div className="mt-1 flex items-center gap-2">
          <code className="font-mono">{id}</code>
          <Button
            type="button"
            size="icon-xs"
            variant="ghost"
            aria-label={t('request.copyId')}
            onClick={() => {
              void navigator.clipboard
                .writeText(id)
                .then(() =>
                  toast.add({ title: t('request.copied'), type: 'success' }),
                )
                .catch(() => undefined)
            }}
          >
            <CopyGlassIcon size="1.4em" aria-hidden="true" />
          </Button>
        </div>
      ) : null}
    </div>
  )
}

export function SignInRequestCard({
  request,
  compact = false,
  gone = false,
}: {
  request: SignInApprovalRequest
  compact?: boolean
  gone?: boolean
}) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const decide = useDecideApproval()
  const [outcome, setOutcome] = useState<Outcome | null>(null)
  const [error, setError] = useState('')
  const [value, setValue] = useState('')
  const [choosing, setChoosing] = useState(false)
  const [byPassword, setByPassword] = useState(false)
  const [password, setPassword] = useState('')
  const [totp, setTotp] = useState('')
  const canUsePassword = ipScope(request.requester_ip) === 'this-computer'
  const now = useNow(outcome === null && !gone)
  const left = secondsLeft(request.expires_at, now)
  const device = deviceTitle(request.user_agent, t)

  const state: Outcome | null =
    outcome ??
    (gone ? (error ? 'failed' : 'handled') : left === 0 ? 'expired' : null)

  const act = (approve: boolean) => {
    setError('')
    decide.mutate(
      byPassword && approve
        ? { id: request.id, approve, password, code: totp }
        : {
            id: request.id,
            approve,
            match: approve ? Number(value) : undefined,
          },
      {
        onSuccess: () => {
          setOutcome(approve ? 'approved' : 'denied')
          if (compact) {
            toast.add({
              title: approve ? t('requests.approved') : t('requests.denied'),
              type: 'success',
            })
          }
        },
        onError: (err) => {
          if (err instanceof ApiError && GONE_STATUSES.has(err.status)) {
            setOutcome('handled')
            return
          }
          setError(err.message || t('requests.actionFailed'))
          setValue('')
          setPassword('')
        },
      },
    )
  }

  const body = (() => {
    switch (state) {
      case 'approved':
        return (
          <Result
            tone="success"
            icon={<CircleCheckGlassIcon className="size-6 text-tone-success" />}
            title={t('request.approvedTitle', { device })}
          >
            <p>{t('request.approvedBody')}</p>
          </Result>
        )
      case 'denied':
        return (
          <Result
            tone="neutral"
            icon={<CircleXGlassIcon className="size-6 text-tone-danger" />}
            title={t('request.deniedTitle')}
          >
            <Link to="/security" className="underline underline-offset-2">
              {t('request.reviewSecurity')}
            </Link>
          </Result>
        )
      case 'expired':
        return (
          <Result
            tone="neutral"
            icon={<CountdownGlassIcon className="size-6" />}
            title={t('request.expiredTitle')}
          >
            <p>{t('request.expiredBody')}</p>
          </Result>
        )
      case 'handled':
        return (
          <Result
            tone="neutral"
            icon={<InfoGlassIcon className="size-6 text-tone-info" />}
            title={t('request.handledTitle')}
          >
            <p>{t('request.handledBody')}</p>
          </Result>
        )
      case 'failed':
        return (
          <Result
            tone="danger"
            icon={<ShieldAlertGlassIcon className="size-6 text-tone-warning" />}
            title={t('request.failedTitle')}
          >
            <p>{error}</p>
          </Result>
        )
      default:
        return null
    }
  })()

  const showNumber = !compact || choosing
  const ready = byPassword ? password !== '' : isValidMatch(value)
  const errorId = `signin-err-${request.id}`
  // Compact starts as one slim row; Approve expands it to the number entry.
  const slim = compact && body === null && !choosing

  const actions = (
    <div
      className={cn(
        'flex items-center gap-2',
        compact ? (slim ? 'ml-auto' : null) : 'flex-col-reverse sm:flex-row',
      )}
    >
      <Button
        type="button"
        variant="secondary"
        size={compact ? 'sm' : 'default'}
        className={cn(
          PRESS,
          'shadow-sm ring-1 ring-inset ring-foreground/10 sm:min-w-20',
        )}
        disabled={decide.isPending}
        onClick={() => act(false)}
      >
        {t('requests.deny')}
      </Button>
      <Button
        type="button"
        size={compact ? 'sm' : 'default'}
        className={cn(
          PRESS,
          'shadow-md ring-1 ring-inset ring-white/20 sm:min-w-20',
        )}
        disabled={decide.isPending || (showNumber && !ready)}
        onClick={() => (showNumber ? act(true) : setChoosing(true))}
      >
        {t('requests.approve')}
      </Button>
      {compact ? null : (
        <button
          type="button"
          className="rounded-sm text-sm text-muted-foreground underline underline-offset-2 outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring sm:ml-auto"
          disabled={decide.isPending}
          onClick={() => act(false)}
        >
          {t('request.notMe')}
        </button>
      )}
    </div>
  )

  const header = (
    <header className="flex items-center gap-2.5">
      <span
        aria-hidden="true"
        className="grid size-8 shrink-0 place-items-center rounded-lg bg-linear-to-b from-tone-warning-soft to-tone-warning-soft/60 text-tone-warning shadow-sm ring-1 ring-inset ring-tone-warning-border"
      >
        <ShieldCheckGlassIcon className="size-5 text-tone-success" />
      </span>
      <div className="min-w-0">
        <h3 className="text-sm leading-tight font-semibold">
          {t('request.headline')}
        </h3>
        {compact ? null : (
          <p className="text-xs leading-tight text-muted-foreground">
            {t('request.subtitle')}
          </p>
        )}
      </div>
    </header>
  )

  return (
    <article
      aria-label={t('request.cardLabel', { device })}
      className={cn(
        'rounded-xl bg-linear-to-b from-card to-muted/50 text-card-foreground shadow-card ring-1 ring-foreground/10',
        compact ? 'space-y-2 p-2.5' : 'space-y-3 p-4',
      )}
    >
      {compact ? (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          {header}
          <SignInRequestFacts request={request} now={now} compact />
          {slim ? actions : null}
        </div>
      ) : (
        <>
          {header}
          <SignInRequestFacts request={request} now={now} compact={false} />
        </>
      )}
      {slim
        ? null
        : (body ?? (
            <div
              className={
                compact
                  ? 'flex flex-wrap items-end gap-x-4 gap-y-2'
                  : 'space-y-2.5'
              }
            >
              <p
                className={cn(
                  'text-xs text-muted-foreground',
                  compact && 'basis-full',
                )}
              >
                {t('request.risk')}
              </p>
              {showNumber ? (
                <div className="space-y-1.5">
                  {byPassword ? (
                    <div className="max-w-sm space-y-1.5">
                      <label
                        htmlFor={`signin-pw-${request.id}`}
                        className="text-xs font-medium"
                      >
                        {t('request.passwordLabel')}
                      </label>
                      <Input
                        id={`signin-pw-${request.id}`}
                        type="password"
                        autoComplete="current-password"
                        value={password}
                        aria-invalid={error !== '' ? true : undefined}
                        aria-describedby={error ? errorId : undefined}
                        onChange={(e) => {
                          setPassword(e.target.value)
                          setError('')
                        }}
                      />
                      {/authenticator/i.test(error) ? (
                        <>
                          <label
                            htmlFor={`signin-totp-${request.id}`}
                            className="text-xs font-medium"
                          >
                            {t('request.codeLabel')}
                          </label>
                          <Input
                            id={`signin-totp-${request.id}`}
                            inputMode="numeric"
                            autoComplete="one-time-code"
                            value={totp}
                            onChange={(e) => {
                              setTotp(e.target.value)
                            }}
                          />
                        </>
                      ) : null}
                      <p className="text-xs text-muted-foreground">
                        {t('request.passwordNote')}
                      </p>
                    </div>
                  ) : (
                    <>
                      <p
                        id={`signin-prompt-${request.id}`}
                        className="text-xs font-medium"
                      >
                        {t('request.numberPrompt')}
                      </p>
                      <MatchDigits
                        value={value}
                        onChange={(next) => {
                          setValue(next)
                          setError('')
                        }}
                        invalid={error !== ''}
                        describedBy={error ? errorId : undefined}
                      />
                    </>
                  )}
                  {error ? (
                    <p
                      id={errorId}
                      role="alert"
                      className="text-sm text-destructive"
                    >
                      {error}
                    </p>
                  ) : null}
                  {canUsePassword ? (
                    <button
                      type="button"
                      className="rounded-sm text-xs text-muted-foreground underline underline-offset-2 outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                      onClick={() => {
                        setByPassword((v) => !v)
                        setError('')
                      }}
                    >
                      {byPassword
                        ? t('request.useNumber')
                        : t('request.usePassword')}
                    </button>
                  ) : null}
                </div>
              ) : null}
              {actions}
            </div>
          ))}
      {compact ? null : <RequestDetails id={request.id} />}
    </article>
  )
}
