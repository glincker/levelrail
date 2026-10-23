import { useTranslation } from 'react-i18next'
import { ArrowSquareOutIcon } from '@phosphor-icons/react/dist/ssr'
import { TOKEN_GUIDES, type TokenHost } from '../../lib/gitTokenGuides'

// The numbered steps for creating a token on one host, with the real scope names to tick.
export function GitTokenGuide({
  host,
  serverUrl,
  tokenName,
}: Readonly<{ host: TokenHost; serverUrl: string; tokenName: string }>) {
  const { t } = useTranslation('settings')
  const guide = TOKEN_GUIDES[host]
  const url = guide.selfHosted ? serverUrl.trim() : guide.defaultUrl
  const page = guide.tokenPage(url || guide.defaultUrl, tokenName)
  const canOpen = url !== ''

  return (
    <section
      aria-label={t('gitConnections.token.guideLabel')}
      className="space-y-3 rounded-md border border-border bg-muted/30 p-3 text-sm"
    >
      {guide.selfHosted ? (
        <p className="text-muted-foreground">
          {t('gitConnections.token.server')}{' '}
          <span className="break-all font-mono text-foreground">
            {url || t('gitConnections.token.serverEmpty')}
          </span>
        </p>
      ) : null}
      <ol className="list-decimal space-y-1.5 pl-5">
        <li>
          {t(`gitConnections.token.steps.${host}.open`)}{' '}
          {canOpen ? (
            <a
              href={page}
              target="_blank"
              rel="noreferrer"
              className="inline-flex min-w-0 items-center gap-1 break-all font-mono text-xs text-primary underline-offset-2 hover:underline"
            >
              {page}
              <ArrowSquareOutIcon
                className="size-3.5 shrink-0"
                aria-hidden="true"
              />
            </a>
          ) : null}
        </li>
        <li>
          {t(`gitConnections.token.steps.${host}.create`, { name: tokenName })}
        </li>
        <li>
          {t('gitConnections.token.tick')}
          <ul
            className="mt-1 flex flex-wrap gap-1.5"
            aria-label={t('gitConnections.token.scopes')}
          >
            {guide.scopes.map((scope) => (
              <li key={scope}>
                <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                  {scope}
                </code>
              </li>
            ))}
          </ul>
        </li>
        <li>{t(`gitConnections.token.steps.${host}.copy`)}</li>
      </ol>
      <p className="text-xs text-muted-foreground">
        {t('gitConnections.token.noDomain')}
      </p>
    </section>
  )
}
