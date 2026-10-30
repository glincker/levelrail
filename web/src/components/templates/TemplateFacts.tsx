import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon } from '@thesvg/ui-react'
import { formatBytes } from '../../lib/format'
import { registryLinkForImage } from '../../lib/imageRegistry'
import type { ComposePreviewService } from '../../types/compose'

function uniq(values: string[]): string[] {
  return Array.from(new Set(values))
}

function Row({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1">
      <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {label}
      </dt>
      <dd className="text-sm text-foreground">{children}</dd>
    </div>
  )
}

const EXTERNAL_LINK =
  'inline-flex items-center gap-1 text-primary underline underline-offset-4'

/** Facts panel built only from the template data and its compose preview; absent fields are omitted. */
export function TemplateFacts({
  services,
  documentationUrl,
  recommendedMemoryBytes,
}: {
  services: ComposePreviewService[]
  documentationUrl: string
  recommendedMemoryBytes?: number
}) {
  const { t } = useTranslation('templates')
  const images = uniq(
    services.map((s) => s.image ?? '').filter((i) => i !== ''),
  )
  const ports = uniq(
    services.map((s) => (s.port ? String(s.port) : '')).filter((p) => p !== ''),
  )
  const volumes = uniq(services.flatMap((s) => s.volumes ?? []))

  return (
    <section
      aria-labelledby="template-facts"
      className="rounded-lg border border-border bg-card p-4"
    >
      <h2 id="template-facts" className="mb-3 text-sm font-medium">
        {t('detail.facts.title')}
      </h2>
      <dl className="space-y-4">
        {images.length > 0 ? (
          <Row label={t('detail.facts.images')}>
            <ul className="space-y-1.5">
              {images.map((image) => {
                const link = registryLinkForImage(image)
                return (
                  <li key={image} className="min-w-0">
                    <span className="block truncate font-mono text-xs">
                      {image}
                    </span>
                    {link ? (
                      <a
                        href={link.url}
                        target="_blank"
                        rel="noreferrer noopener"
                        className={`${EXTERNAL_LINK} text-xs`}
                        aria-label={t('detail.facts.registryLink', {
                          image,
                          registry: link.registry,
                        })}
                      >
                        {link.registry}
                        <ExternalLinkGlassIcon
                          className="size-4.5"
                          aria-hidden="true"
                        />
                      </a>
                    ) : null}
                  </li>
                )
              })}
            </ul>
          </Row>
        ) : null}
        {documentationUrl ? (
          <Row label={t('detail.facts.docs')}>
            <a
              href={documentationUrl}
              target="_blank"
              rel="noreferrer noopener"
              className={EXTERNAL_LINK}
            >
              {t('detail.facts.docsLink')}
              <ExternalLinkGlassIcon className="size-5" aria-hidden="true" />
            </a>
          </Row>
        ) : null}
        {recommendedMemoryBytes ? (
          <Row label={t('detail.facts.memory')}>
            {formatBytes(recommendedMemoryBytes)}
          </Row>
        ) : null}
        {ports.length > 0 ? (
          <Row label={t('detail.facts.ports')}>
            <span className="font-mono text-xs">{ports.join(', ')}</span>
          </Row>
        ) : null}
        {volumes.length > 0 ? (
          <Row label={t('detail.facts.volumes')}>
            <ul className="space-y-0.5">
              {volumes.map((v) => (
                <li key={v} className="truncate font-mono text-xs">
                  {v}
                </li>
              ))}
            </ul>
          </Row>
        ) : null}
      </dl>
    </section>
  )
}
