import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'
import { httpsStatusQueryOptions } from '../../queries/httpsStatus'

const DEFAULT_PORT = '8080'

const LINES = ['deploy', 'protect', 'time'] as const

/** WelcomeStep says what the product does and how long setup takes; the footer carries Start and Skip. */
export function WelcomeStep({
  busy,
  onDismiss,
}: {
  busy: boolean
  onDismiss: () => void
}) {
  const { t } = useTranslation('onboarding')
  const https = useQuery({ ...httpsStatusQueryOptions(), retry: false })
  // Only a known-private dashboard gets the tunnel: an issued certificate or
  // a reported public URL means a plain browser visit works.
  const isPrivate =
    https.data !== undefined &&
    https.data.state !== 'issued' &&
    !https.data.dashboard_url
  const port = window.location.port || DEFAULT_PORT
  const host = https.data?.public_host || t('welcome.tunnelHost')
  const tunnel = `ssh -L ${port}:127.0.0.1:${port} ${t('welcome.tunnelUser')}@${host}`
  return (
    <div className="space-y-4">
      <ul className="space-y-2.5">
        {LINES.map((line) => (
          <li key={line} className="flex items-start gap-2.5 text-sm">
            <CircleCheckGlassIcon
              aria-hidden="true"
              className="mt-0.5 size-4.5 shrink-0 text-tone-success"
            />
            {t(`welcome.${line}`)}
          </li>
        ))}
      </ul>
      {isPrivate ? (
        <div className="space-y-1.5" data-testid="welcome-tunnel">
          <p className="text-xs text-muted-foreground">
            {t('welcome.tunnelHint', { port })}
          </p>
          <CodeBlock code={tunnel} label={t('welcome.tunnelLabel')} />
        </div>
      ) : null}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={busy}
        onClick={onDismiss}
      >
        {t('welcome.dismiss')}
      </Button>
    </div>
  )
}
