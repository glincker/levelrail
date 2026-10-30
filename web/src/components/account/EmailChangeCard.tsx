import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SettingsCardSkeleton } from '../settings/SettingsSkeletons'
import {
  emailChangeStatusQueryOptions,
  useConfirmEmailChange,
  useRequestEmailChange,
  type AccountApiError,
} from '../../queries/accountSelf'
import { useSession } from '../../queries/security'

const CONFIRM_PARAM = 'confirm_email'

function readConfirmToken(): string {
  return (
    new URLSearchParams(window.location.hash.replace(/^#/, '')).get(
      CONFIRM_PARAM,
    ) ?? ''
  )
}

function clearConfirmParam(): void {
  const url = new URL(window.location.href)
  url.hash = ''
  window.history.replaceState(window.history.state, '', url)
}

type TFn = ReturnType<
  typeof import('react-i18next').useTranslation<'account'>
>['t']

function requestErrorText(error: AccountApiError, t: TFn): string {
  switch (error.code) {
    case 'step_up_locked':
      return t('email.locked')
    case 'step_up_invalid':
      return t('email.invalid')
    case 'email_not_configured':
      return t('email.notConfigured')
    case 'email_change_limited':
      return t('email.rateLimited')
    default:
      return error.status === 400 ? t('email.badAddress') : error.message
  }
}

function ConfirmBanner({ token }: { token: string }) {
  const { t } = useTranslation('account')
  const confirm = useConfirmEmailChange()
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true
    confirm.mutate(token, { onSettled: clearConfirmParam })
  }, [confirm, token])

  if (confirm.isPending || confirm.isIdle) {
    return (
      <p className="text-sm text-muted-foreground" role="status">
        {t('email.confirming')}
      </p>
    )
  }
  if (confirm.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
        <AlertDescription>
          {confirm.error.code === 'email_taken'
            ? t('email.emailTaken')
            : t('email.confirmFailed')}
        </AlertDescription>
      </Alert>
    )
  }
  return (
    <Alert>
      <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
      <AlertDescription>
        {t('email.confirmed', { email: confirm.data.email })}
      </AlertDescription>
    </Alert>
  )
}

export function EmailChangeCard() {
  const { t } = useTranslation('account')
  const { data: session } = useSession()
  const status = useQuery(emailChangeStatusQueryOptions())
  const request = useRequestEmailChange()
  const [token] = useState(readConfirmToken)
  const [newEmail, setNewEmail] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [needCode, setNeedCode] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    request.mutate(
      { new_email: newEmail.trim(), password, code: code || undefined },
      {
        onSuccess: () => {
          setPassword('')
          setCode('')
        },
        onError: (err) => {
          if (err.code === 'step_up_code_required') {
            setNeedCode(true)
            setError(t('email.codeHint'))
            return
          }
          setError(requestErrorText(err, t))
        },
      },
    )
  }

  if (status.isPending) return <SettingsCardSkeleton rows={2} />

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('email.title')}</CardTitle>
        <CardDescription>{t('email.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {token !== '' ? <ConfirmBanner token={token} /> : null}
        <div className="space-y-1 text-sm">
          <p className="text-xs font-medium text-muted-foreground">
            {t('email.current')}
          </p>
          <p>{session.username}</p>
        </div>
        {status.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription className="flex items-center justify-between gap-3">
              <span>{t('email.statusFailed')}</span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void status.refetch()}
              >
                {t('common.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : !status.data.available ? (
          <Alert>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
              <span>{t('email.notConfigured')}</span>
              <Link
                to="/settings/email"
                className={buttonVariants({ variant: 'outline', size: 'sm' })}
              >
                {t('email.configure')}
              </Link>
            </AlertDescription>
          </Alert>
        ) : !session.has_password ? (
          <Alert>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription>{t('email.passwordRequired')}</AlertDescription>
          </Alert>
        ) : request.isSuccess ? (
          <Alert>
            <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
            <AlertDescription>{t('email.sent')}</AlertDescription>
          </Alert>
        ) : (
          <form className="max-w-md space-y-3" onSubmit={submit}>
            <div className="space-y-1.5">
              <Label htmlFor="new-email">{t('email.newEmail')}</Label>
              <Input
                id="new-email"
                type="email"
                autoComplete="email"
                required
                value={newEmail}
                onChange={(e) => setNewEmail(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="new-email-password">{t('email.password')}</Label>
              <Input
                id="new-email-password"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            {needCode ? (
              <div className="space-y-1.5">
                <Label htmlFor="new-email-code">{t('email.code')}</Label>
                <Input
                  id="new-email-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                />
              </div>
            ) : null}
            {error ? (
              <Alert variant="destructive" role="alert">
                <TriangleAlertGlassIcon
                  className="text-tone-warning"
                  size="1.7em"
                />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            <Button
              type="submit"
              size="sm"
              disabled={request.isPending}
              writeAction
            >
              {request.isPending ? t('email.sending') : t('email.send')}
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  )
}
