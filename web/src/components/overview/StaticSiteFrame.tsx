import { useEffect, useState } from 'react'
import { SiteThumbnail } from './SiteThumbnail'
import { cn } from '@/lib/utils'

export type FrameDevice = 'desktop' | 'mobile'

type Probe = 'checking' | 'ok' | 'blocked'

/** Opaque no-cors fetch: it only rejects when the site cannot be reached or trusted. */
function useReachable(url: string): Probe {
  const [state, setState] = useState<{ url: string; probe: Probe }>({
    url,
    probe: 'checking',
  })
  useEffect(() => {
    const ctl = new AbortController()
    fetch(url, { mode: 'no-cors', signal: ctl.signal }).then(
      () => setState({ url, probe: 'ok' }),
      () => {
        if (!ctl.signal.aborted) setState({ url, probe: 'blocked' })
      },
    )
    return () => ctl.abort()
  }, [url])
  return state.url === url ? state.probe : 'checking'
}

/**
 * A scaled, non-interactive live view of the site. The iframe is laid out at
 * twice the card width and halved, so no JS measuring or inline style is needed.
 */
export function StaticSiteFrame({
  url,
  device,
  title,
  name,
  deployedAt,
}: {
  url: string
  device: FrameDevice
  title: string
  name: string
  deployedAt?: string
}) {
  const probe = useReachable(url)
  const mobile = device === 'mobile'
  return (
    <div
      className={cn(
        'relative aspect-[8/5] w-full overflow-hidden rounded-md border border-border bg-muted',
      )}
    >
      {probe === 'blocked' ? (
        <SiteThumbnail url={url} title={name} deployedAt={deployedAt} />
      ) : probe === 'ok' ? (
        <div
          className={cn(
            'relative h-full overflow-hidden bg-background',
            mobile ? 'mx-auto w-48 border-x border-border' : 'w-full',
          )}
          aria-hidden="true"
        >
          <iframe
            src={url}
            title={title}
            sandbox=""
            loading="lazy"
            tabIndex={-1}
            referrerPolicy="no-referrer"
            className="pointer-events-none h-[200%] w-[200%] origin-top-left scale-50 border-0 bg-white"
          />
        </div>
      ) : (
        <div className="absolute inset-0 animate-pulse bg-muted motion-reduce:animate-none" />
      )}
    </div>
  )
}
