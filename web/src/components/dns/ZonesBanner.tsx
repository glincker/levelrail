import type { ReactNode } from 'react'
import { GlobeGlassIcon } from '@thesvg/ui-react'
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
      icon={<GlobeGlassIcon size="1.4em" />}
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
