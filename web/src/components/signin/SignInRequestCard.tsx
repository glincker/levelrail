import { useState, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  CheckCircleIcon,
  ClockCountdownIcon,
  CopyIcon,
  InfoIcon,
  ShieldCheckIcon,
  ShieldWarningIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { ApiError } from '../../lib/apiError'
import { isValidMatch, secondsLeft } from '../../lib/signInRequest'
import {
  useDecideApproval,
  type SignInApprovalRequest,
} from '../../queries/signIn'
import { MatchDigits } from './MatchDigits'
import { SignInRequestFacts } from './SignInRequestFacts'
import { deviceTitle, useNow } from './signInRequestHooks'

type Outcome = 'approved' | 'denied' | 'handled' | 'expired' | 'failed'

const GONE_STATUSES = new Set([404, 409, 410])

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
        <CaretDownIcon
          aria-hidden="true"
          className={cn('size-3 transition-transform', open && 'rotate-180')}
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
            <CopyIcon aria-hidden="true" />
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
  const now = useNow(outcome === null && !gone)
  const left = secondsLeft(request.expires_at, now)
  const device = deviceTitle(request.user_agent, t)

  const state: Outcome | null =
    outcome ??
    (gone ? (error ? 'failed' : 'handled') : left === 0 ? 'expired' : null)

  const act = (approve: boolean) => {
    setError('')
    decide.mutate(
      { id: request.id, approve, match: approve ? Number(value) : undefined },
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
            icon={<CheckCircleIcon className="size-5" />}
            title={t('request.approvedTitle', { device })}
          >
            <p>{t('request.approvedBody')}</p>
          </Result>
        )
      case 'denied':
        return (
          <Result
            tone="neutral"
            icon={<XCircleIcon className="size-5" />}
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
            icon={<ClockCountdownIcon className="size-5" />}
            title={t('request.expiredTitle')}
          >
            <p>{t('request.expiredBody')}</p>
          </Result>
        )
      case 'handled':
        return (
          <Result
            tone="neutral"
            icon={<InfoIcon className="size-5" />}
            title={t('request.handledTitle')}
          >
            <p>{t('request.handledBody')}</p>
          </Result>
        )
      case 'failed':
        return (
          <Result
            tone="danger"
            icon={<ShieldWarningIcon className="size-5" />}
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
  const ready = isValidMatch(value)
  const errorId = `signin-err-${request.id}`

  return (
    <article
      aria-label={t('request.cardLabel', { device })}
      className={cn(
        'space-y-4 rounded-xl border border-border bg-card text-card-foreground',
        compact ? 'p-3' : 'p-4 sm:p-5',
      )}
    >
      <header className="flex items-center gap-3">
        <span
          aria-hidden="true"
          className="grid size-10 shrink-0 place-items-center rounded-lg bg-tone-warning-soft text-tone-warning"
        >
          <ShieldCheckIcon className="size-5" />
        </span>
        <div className="min-w-0">
          <h3 className={cn('font-semibold', compact ? 'text-sm' : 'text-lg')}>
            {t('request.headline')}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t('request.subtitle')}
          </p>
        </div>
      </header>
      <SignInRequestFacts request={request} now={now} compact={compact} />
      {body ?? (
        <div className="space-y-3">
          <p className="text-sm text-muted-foreground">{t('request.risk')}</p>
          {showNumber ? (
            <div className="space-y-2">
              <p
                id={`signin-prompt-${request.id}`}
                className="text-sm font-medium"
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
              {error ? (
                <p
                  id={errorId}
                  role="alert"
                  className="text-sm text-destructive"
                >
                  {error}
                </p>
              ) : null}
            </div>
          ) : null}
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center">
            <Button
              type="button"
              size="lg"
              variant="secondary"
              className="sm:min-w-28"
              disabled={decide.isPending}
              onClick={() => act(false)}
            >
              {t('requests.deny')}
            </Button>
            <Button
              type="button"
              size="lg"
              className="sm:min-w-28"
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
        </div>
      )}
      {compact ? null : <RequestDetails id={request.id} />}
    </article>
  )
}
