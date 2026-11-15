import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  ExternalLinkGlassIcon,
  KeyGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  GIT_CONNECT_CONFIG,
  gitAppPageUrl,
  gitRedirectHint,
  gitRedirectUri,
  type GitConnectProvider,
} from '../../lib/gitHost'

// Guided OAuth steps for one provider: where to register the application,
// the exact redirect URI and scopes, and a token fallback so nobody is stuck.
export function GitConnectGuide({
  provider,
  baseUrl,
  origin = window.location.origin,
  onUseToken,
}: Readonly<{
  provider: GitConnectProvider
  baseUrl: string
  origin?: string
  onUseToken?: () => void
}>) {
  const { t } = useTranslation('gitConnect')
  const [copied, setCopied] = useState<'idle' | 'done' | 'failed'>('idle')
  const config = GIT_CONNECT_CONFIG[provider]
  if (!config.oauth) return null

  const redirect = gitRedirectUri(origin, provider)
  const hint = gitRedirectHint(origin)
  const appPage = gitAppPageUrl(provider, baseUrl)
  const host = (() => {
    try {
      return new URL(appPage ?? '').host
    } catch {
      return ''
    }
  })()
  const providerName = t(`name.${provider}`)

  return (
    <section
      aria-label={t('title', { provider: providerName })}
      className="space-y-3 rounded-md border border-border bg-muted/30 p-3 text-sm"
    >
      <ol className="list-decimal space-y-3 pl-5 marker:text-muted-foreground">
        <li className="space-y-1">
          <p>{t('stepApp')}</p>
          {appPage ? (
            <a
              href={appPage}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex max-w-full items-center gap-1 break-all text-primary underline-offset-2 hover:underline"
            >
              {t('openApp', { host })}
              <ExternalLinkGlassIcon
                className="size-5 shrink-0"
                aria-hidden="true"
              />
            </a>
          ) : null}
          <p className="text-xs text-muted-foreground">
            {t(`appHint.${provider}`)}
          </p>
        </li>
        <li className="space-y-1.5">
          <p>{t('stepRedirect')}</p>
          <div className="flex flex-wrap items-center gap-2">
            <span className="sr-only">{t('redirectLabel')}</span>
            <code
              aria-label={t('redirectLabel')}
              className="min-w-0 flex-1 break-all rounded bg-background px-2 py-1 text-xs"
            >
              {redirect}
            </code>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                void navigator.clipboard.writeText(redirect).then(
                  () => setCopied('done'),
                  () => setCopied('failed'),
                )
              }}
            >
              {copied === 'done' ? (
                <CheckGlassIcon className="size-5" aria-hidden="true" />
              ) : (
                <CopyGlassIcon className="size-5" aria-hidden="true" />
              )}
              {copied === 'done' ? t('copied') : t('copy')}
            </Button>
          </div>
          <p role="status" className="text-xs text-destructive empty:hidden">
            {copied === 'failed' ? t('copyFailed') : ''}
          </p>
          <p
            data-hint={hint}
            className={
              hint === 'insecure'
                ? 'text-xs text-destructive'
                : 'text-xs text-muted-foreground'
            }
          >
            {t(`hint.${hint}`)}
          </p>
        </li>
        <li className="space-y-1">
          <p>{t('stepScopes')}</p>
          {config.scopes.length > 0 ? (
            <ul
              className="flex flex-wrap gap-1.5"
              aria-label={t('scopesLabel')}
            >
              {config.scopes.map((scope) => (
                <li key={scope}>
                  <code className="rounded bg-background px-1.5 py-0.5 text-xs">
                    {scope}
                  </code>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-xs text-muted-foreground">{t('scopesNone')}</p>
          )}
        </li>
        <li>{t('stepPaste')}</li>
      </ol>
      {onUseToken ? (
        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-3">
          <p className="min-w-0 flex-1 text-xs text-muted-foreground">
            <span className="font-medium text-foreground">
              {t('fallback.title')}
            </span>{' '}
            {t('fallback.body')}
          </p>
          <Button
            type="button"
            size="sm"
            variant={hint === 'insecure' ? 'default' : 'outline'}
            onClick={onUseToken}
          >
            <KeyGlassIcon className="size-5" aria-hidden="true" />
            {t('fallback.action')}
          </Button>
        </div>
      ) : null}
    </section>
  )
}
