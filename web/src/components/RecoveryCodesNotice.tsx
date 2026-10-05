import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { KeyIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { twoFactorStatusQueryOptions } from '../queries/twoFactor'

const SECURITY_PATH = '/settings/security'
const DISMISS_KEY = 'recovery-codes-notice-dismissed'
const STATUS_STALE_MS = 5 * 60 * 1000

function readDismissed(): boolean {
  try {
    return window.sessionStorage.getItem(DISMISS_KEY) === '1'
  } catch {
    return false
  }
}

function writeDismissed(): void {
  try {
    window.sessionStorage.setItem(DISMISS_KEY, '1')
  } catch {
    // Dismissal just won't persist for this tab session.
  }
}

interface RecoveryCodesNoticeProps {
  // inline drops the link and dismiss button, for use on the page that already regenerates.
  inline?: boolean
}

export function RecoveryCodesNotice({ inline }: RecoveryCodesNoticeProps) {
  const { t } = useTranslation('settings')
  const [dismissed, setDismissed] = useState(readDismissed)
  const onSecurityPage = useRouterState({
    select: (s) => s.location.pathname === SECURITY_PATH,
  })
  const { data } = useQuery({
    ...twoFactorStatusQueryOptions(),
    staleTime: STATUS_STALE_MS,
    refetchOnWindowFocus: false,
  })

  if (!data?.recovery_codes_need_regeneration) return null
  if (!inline && (dismissed || onSecurityPage)) return null

  return (
    <Alert className="relative">
      <KeyIcon />
      <AlertTitle>{t('recoveryCodesNotice.title')}</AlertTitle>
      <AlertDescription>
        {t('recoveryCodesNotice.description')}
        {inline ? null : (
          <div className="mt-2">
            <Button
              size="sm"
              render={<Link to={SECURITY_PATH} />}
              nativeButton={false}
            >
              {t('recoveryCodesNotice.action')}
            </Button>
          </div>
        )}
      </AlertDescription>
      {inline ? null : (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="absolute top-1.5 right-1.5"
          aria-label={t('recoveryCodesNotice.dismiss')}
          onClick={() => {
            writeDismissed()
            setDismissed(true)
          }}
        >
          <XIcon />
        </Button>
      )}
    </Alert>
  )
}
