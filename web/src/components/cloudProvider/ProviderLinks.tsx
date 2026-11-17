import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon } from '@thesvg/ui-react'
import type { CloudProviderPresentation } from '../../lib/cloudProviderBrand'
import type { ProviderLabels } from './useProviderLabels'

// Plain external anchors: these are the provider's own pages, not Levelrail docs.
export function ProviderLinks({
  provider,
  labels,
}: Readonly<{
  provider: CloudProviderPresentation
  labels: ProviderLabels
}>) {
  const { t } = useTranslation('cloudProvider')
  const links = [
    {
      href: provider.consoleUrl,
      text: t('links.console', { name: labels.name }),
    },
    { href: provider.tokenUrl, text: t('links.token') },
    { href: provider.docsUrl, text: t('links.docs') },
  ].filter((l) => l.href)
  if (links.length === 0) return null
  return (
    <nav aria-label={t('links.label')}>
      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
        {links.map((l) => (
          <li key={l.href}>
            <a
              href={l.href}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
            >
              {l.text}
              <ExternalLinkGlassIcon size="1.2em" aria-hidden="true" />
            </a>
          </li>
        ))}
      </ul>
    </nav>
  )
}
