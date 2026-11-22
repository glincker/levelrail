import { useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { BrowserPushField } from '@/components/BrowserPushField'
import { useIsMobile } from '../hooks/use-mobile'
import {
  canPromptInstall,
  dismissInstallCard,
  installCardDismissed,
  isIOSSafari,
  isStandalone,
  promptInstall,
  subscribeInstallPrompt,
} from '../lib/pwaInstall'

// The push field reports readiness for its own button; the card has nothing to do with it.
const ignoreReady = () => undefined

// Shown on a phone after sign in: step one adds the app to the home screen, step two (once
// installed, or in the installed app) asks for push permission through the existing push field.
export function InstallCard() {
  const { t } = useTranslation('installCard')
  const isMobile = useIsMobile()
  const canPrompt = useSyncExternalStore(
    subscribeInstallPrompt,
    canPromptInstall,
  )
  const dismissed = useSyncExternalStore(
    subscribeInstallPrompt,
    installCardDismissed,
  )
  if (!isMobile || dismissed) return null
  const standalone = isStandalone()
  const ios = isIOSSafari()
  if (!standalone && !canPrompt && !ios) return null

  return (
    <Card className="mx-3 mt-2">
      <CardContent className="space-y-3">
        <p className="text-sm font-semibold">
          {standalone ? t('pushTitle') : t('title')}
        </p>
        {standalone ? (
          <>
            <p className="text-sm text-muted-foreground">{t('pushHint')}</p>
            <BrowserPushField onReadyChange={ignoreReady} />
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            {canPrompt ? t('install') : t('iosSteps')}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          {!standalone && canPrompt ? (
            <Button
              type="button"
              size="sm"
              onClick={() => {
                void promptInstall()
              }}
            >
              {t('installButton')}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={dismissInstallCard}
          >
            {t('dismiss')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
