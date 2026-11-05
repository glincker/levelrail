import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArchiveRestoreGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { onboardingActive, useMeOnboarding } from '../../queries/meOnboarding'

const DISMISS_KEY = 'dashboard.restoreDue.dismissed.v1'

function readDismissed(): boolean {
  try {
    return window.localStorage.getItem(DISMISS_KEY) === '1'
  } catch {
    return false
  }
}

/** RestoreDueCard nudges a first restore test once the server says a backup exists but none was verified. */
export function RestoreDueCard() {
  const { t } = useTranslation('dashboard')
  const { data } = useMeOnboarding()
  const [dismissed, setDismissed] = useState(readDismissed)
  if (!onboardingActive(data) || data.restore_due !== true || dismissed) {
    return null
  }
  return (
    <Card>
      <CardContent className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <ArchiveRestoreGlassIcon
          aria-hidden="true"
          size="1.6em"
          className="text-muted-foreground"
        />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-foreground">
            {t('restoreDue.title')}
          </p>
          <p className="text-xs text-muted-foreground">
            {t('restoreDue.body')}
          </p>
        </div>
        <Link
          to="/settings/control-plane-backup"
          className={buttonVariants({ size: 'sm' })}
        >
          {t('restoreDue.action')}
        </Link>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          aria-label={t('restoreDue.dismiss')}
          title={t('restoreDue.dismiss')}
          onClick={() => {
            setDismissed(true)
            try {
              window.localStorage.setItem(DISMISS_KEY, '1')
            } catch {
              // Storage can be blocked; the card then only hides for this visit.
            }
          }}
        >
          <XGlassIcon size="1.4em" />
        </Button>
      </CardContent>
    </Card>
  )
}
