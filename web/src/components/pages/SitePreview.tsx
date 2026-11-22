import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { Skeleton } from '@/components/ui/skeleton'
import { appDetailQueryOptions } from '../../queries/apps'

// Any path no site serves, so the preview shows the site's own 404 page.
const MISSING_PATH = '/__missing-page-preview__'

export type Device = 'desktop' | 'mobile' | 'card'

// A frame that never reports load (self-signed cert, refused connection) falls back after this.
const LOAD_TIMEOUT_MS = Number(
  import.meta.env.VITE_PAGES_PREVIEW_TIMEOUT_MS ?? 8000,
)

// Fixed viewport sizes scaled by fixed factors so no inline style is needed.
const FRAME: Record<Device, { box: string; iframe: string }> = {
  desktop: { box: 'h-50 w-80 max-w-full', iframe: 'h-200 w-320 scale-25' },
  mobile: { box: 'h-50 w-[7.8rem]', iframe: 'h-125 w-97.5 scale-[0.4]' },
  card: { box: 'h-40 w-full', iframe: 'h-200 w-320 scale-20' },
}

export function Placeholder({ url }: Readonly<{ url: string | null }>) {
  const { t } = useTranslation('pagesSites')
  return (
    <div className="flex size-full flex-col items-center justify-center gap-2 p-3 text-center">
      <GlobeGlassIcon
        className="size-6 text-muted-foreground"
        aria-hidden="true"
      />
      <p className="text-xs text-muted-foreground">
        {t('preview.unavailable')}
      </p>
      {url ? (
        <Button
          size="xs"
          variant="outline"
          render={<a href={url} target="_blank" rel="noreferrer" />}
        >
          {t('success.open')}
        </Button>
      ) : null}
    </div>
  )
}

export function PreviewFrame({
  url,
  name,
  device,
}: Readonly<{ url: string; name: string; device: Device }>) {
  const { t } = useTranslation('pagesSites')
  const [loaded, setLoaded] = useState(false)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    const id = window.setTimeout(() => setFailed(true), LOAD_TIMEOUT_MS)
    return () => window.clearTimeout(id)
  }, [])

  if (failed && !loaded) return <Placeholder url={url} />
  return (
    <>
      {loaded ? null : <Skeleton className="absolute inset-0 size-full" />}
      <iframe
        src={url}
        title={t('preview.frame', { name })}
        loading="lazy"
        sandbox="allow-scripts"
        tabIndex={-1}
        aria-hidden="true"
        onLoad={() => setLoaded(true)}
        className={`pointer-events-none absolute top-0 left-0 origin-top-left border-0 bg-white ${FRAME[device].iframe}`}
      />
    </>
  )
}

/** Non-interactive thumbnail of the live site; the live URL may carry a dev port or a self-signed cert. */
export function SitePreview({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const app = useQuery({ ...appDetailQueryOptions(name), retry: false })
  const domain = app.data?.domains?.[0]
  const [device, setDevice] = useState<Exclude<Device, 'card'>>('desktop')
  const [missing, setMissing] = useState(false)
  const base = domain ? `https://${domain}` : null
  const url = base && missing ? `${base}${MISSING_PATH}` : base
  return (
    <section
      aria-label={t('preview.title')}
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold">{t('preview.title')}</h2>
        <div className="flex items-center gap-1">
          <Button
            size="xs"
            variant={missing ? 'secondary' : 'ghost'}
            aria-pressed={missing}
            disabled={!base}
            onClick={() => setMissing((v) => !v)}
          >
            {t('preview.notFoundToggle')}
          </Button>
        </div>
        <SegmentedControl
          aria-label={t('preview.device')}
          value={device}
          onValueChange={setDevice}
          options={[
            { value: 'desktop', label: t('preview.desktop') },
            { value: 'mobile', label: t('preview.mobile') },
          ]}
        />
      </div>
      <div className="flex justify-center rounded-md bg-muted/40 p-1.5">
        <div
          data-testid="preview-frame"
          className={`relative overflow-hidden rounded-md border border-border bg-background ${FRAME[device].box}`}
        >
          {url ? (
            <PreviewFrame
              key={`${url}-${device}`}
              url={url}
              name={name}
              device={device}
            />
          ) : (
            <Placeholder url={null} />
          )}
        </div>
      </div>
    </section>
  )
}
