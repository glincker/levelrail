import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, SettingsGlassIcon } from '@thesvg/ui-react'
import type { OAuthProviderSettings } from '../queries/oauth'
import {
  brandKeyFor,
  callbackUrl,
  connectionState,
} from '../lib/oauthProviders'
import { BrandTile } from './BrandLogoBadge'
import { OAuthProviderForm } from './OAuthProviderForm'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from './ui/sheet'
import { toast } from './ui/toast'

const STATE_BADGE = {
  connected: 'success',
  disabled: 'muted',
  unconfigured: 'outline',
} as const

/** OAuthProviderRow is one provider in the sign-in list; Configure opens its form in a side sheet. */
export function OAuthProviderRow({
  settings,
}: {
  settings: OAuthProviderSettings
}) {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  const state = connectionState(settings)
  const callback = callbackUrl(window.location.origin, settings.provider)
  const label = t(`oauthPage.providers.${settings.provider}`, {
    defaultValue: settings.provider,
  })

  const copyCallback = () => {
    void navigator.clipboard.writeText(callback).then(
      () => {
        toast.add({ title: t('oauthPage.callbackCopied'), type: 'success' })
      },
      () => {
        toast.add({ title: t('oauthPage.callbackCopyFailed'), type: 'error' })
      },
    )
  }

  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
      <BrandTile name={brandKeyFor(settings.provider)} label={label} />
      <span className="w-32 shrink-0 text-sm font-medium">{label}</span>
      <Badge variant={STATE_BADGE[state]}>
        {t(`oauthPage.state.${state}`)}
      </Badge>
      <div className="flex min-w-0 flex-1 basis-64 items-center gap-1">
        <code className="min-w-0 truncate font-mono text-xs text-muted-foreground">
          {callback}
        </code>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t('oauthPage.copyCallbackFor', { provider: label })}
          onClick={copyCallback}
        >
          <CopyGlassIcon size="1.4em" aria-hidden="true" />
        </Button>
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
      >
        <SettingsGlassIcon size="1.4em" aria-hidden="true" />
        {t('oauthPage.configure')}
      </Button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className="data-[side=right]:sm:max-w-xl">
          <SheetHeader className="border-b border-border pr-12">
            <SheetTitle>
              {t('oauthPage.sheet.title', { provider: label })}
            </SheetTitle>
            <SheetDescription>
              {t('oauthPage.sheet.description')}
            </SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
            <OAuthProviderForm settings={settings} label={label} />
          </div>
        </SheetContent>
      </Sheet>
    </li>
  )
}
