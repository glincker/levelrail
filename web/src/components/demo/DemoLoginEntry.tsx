import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { FlaskConicalGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { demoQueryOptions, useDemoSession } from '@/queries/demo'
import { useFinishSignIn } from '@/queries/auth'

const DEFAULT_VISITOR = 'demo'

// An edge widget (Turnstile) injects its token as a hidden field; the API only checks it when configured.
function readHumanToken(): string {
  const el = document.querySelector<HTMLInputElement>(
    'input[name="cf-turnstile-response"]',
  )
  return el?.value ?? ''
}

/** DemoLoginEntry is the one-click entry on the login screen; it renders only on a demo instance. */
export function DemoLoginEntry() {
  const { t } = useTranslation('demo')
  const { data: info } = useQuery(demoQueryOptions())
  const session = useDemoSession()
  const finish = useFinishSignIn()

  if (!info?.enabled) return null

  function errorText(): string | null {
    const err = session.error
    if (!err) return null
    if (err.status === 429) return t('login.rateLimited')
    if (err.status === 403) return t('login.humanCheckFailed')
    return t('login.failed')
  }
  const error = errorText()

  return (
    <div className="space-y-2 rounded-lg border border-border bg-muted/40 p-3">
      <div className="space-y-0.5">
        <p className="text-sm font-medium">{t('login.title')}</p>
        <p className="text-xs text-muted-foreground">
          {t('login.description')}
        </p>
      </div>
      <Button
        type="button"
        className="w-full"
        disabled={session.isPending}
        onClick={() => {
          session.mutate(readHumanToken(), {
            onSuccess: () => {
              finish(info.username ?? DEFAULT_VISITOR)
            },
          })
        }}
      >
        <FlaskConicalGlassIcon size="1.4em" data-icon="inline-start" />
        {session.isPending ? t('login.pending') : t('login.button')}
      </Button>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      {info.username && info.password ? (
        <p className="text-xs text-muted-foreground">
          {t('login.credentials', {
            username: info.username,
            password: info.password,
          })}
        </p>
      ) : null}
    </div>
  )
}
