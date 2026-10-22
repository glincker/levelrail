import type { ReactNode } from 'react'
import { GlobeHemisphereWestIcon } from '@phosphor-icons/react/dist/ssr'
import { HelpLink } from '@/components/HelpLink'
import { PageBanner } from '../PageBanner'
import { LazyDnsDiagram } from '../banners/LazyPass3Diagrams'

/** ZonesBanner is the DNS page header: hero until a provider has zones, compact after. */
export function ZonesBanner({
  empty,
  helpLabel,
  action,
}: Readonly<{ empty: boolean; helpLabel: string; action?: ReactNode }>) {
  const size = empty ? 'hero' : 'compact'
  return (
    <PageBanner
      icon={<GlobeHemisphereWestIcon />}
      titleKey="dns:page.title"
      descriptionKey={empty ? 'dns:banner.description' : 'dns:page.description'}
      action={
        <>
          <HelpLink path="/dns" label={helpLabel} />
          {action}
        </>
      }
      illustration={<LazyDnsDiagram size={size} />}
      size={size}
    />
  )
}
