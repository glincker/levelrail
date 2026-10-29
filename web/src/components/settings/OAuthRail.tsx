import { useTranslation } from 'react-i18next'
import { CopyGlassIcon } from '@thesvg/ui-react'
import {
  PageRail,
  PageRailRow,
  PageRailSection,
} from '@/components/shell/PageRail'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { HelpLink } from '../HelpLink'
import type { OAuthProviderSettings } from '../../queries/oauth'
import { callbackUrl, connectionState } from '../../lib/oauthProviders'

/** OAuthRail summarises sign-in providers and their callback URLs from the settings the page already loads. */
export function OAuthRail({ settings }: { settings: OAuthProviderSettings[] }) {
  const { t } = useTranslation('settings')
  const connected = settings.filter(
    (s) => connectionState(s) === 'connected',
  ).length

  const label = (provider: string): string =>
    t(`oauthPage.providers.${provider}`, { defaultValue: provider })

  const copy = (url: string) => {
    void navigator.clipboard.writeText(url).then(
      () => {
        toast.add({ title: t('oauthPage.callbackCopied'), type: 'success' })
      },
      () => {
        toast.add({ title: t('oauthPage.callbackCopyFailed'), type: 'error' })
      },
    )
  }

  return (
    <PageRail>
      <PageRailSection title={t('oauthPage.rail.summary')}>
        <PageRailRow label={t('oauthPage.rail.connected')}>
          {t('oauthPage.rail.connectedOf', {
            connected,
            total: settings.length,
          })}
        </PageRailRow>
      </PageRailSection>
      <PageRailSection title={t('oauthPage.rail.callbacks')}>
        {settings.map((s) => {
          const url = callbackUrl(window.location.origin, s.provider)
          const name = label(s.provider)
          return (
            <PageRailRow key={s.provider} label={name}>
              <div className="flex min-w-0 items-center gap-1">
                <code className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
                  {url}
                </code>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('oauthPage.rail.copyFor', { provider: name })}
                  onClick={() => {
                    copy(url)
                  }}
                >
                  <CopyGlassIcon size="1.4em" aria-hidden="true" />
                </Button>
              </div>
            </PageRailRow>
          )
        })}
      </PageRailSection>
      <PageRailSection title={t('oauthPage.rail.docs')}>
        <HelpLink
          path="/identity-and-access"
          label={t('oauthPage.rail.docsLink')}
          variant="inline"
        />
      </PageRailSection>
    </PageRail>
  )
}
