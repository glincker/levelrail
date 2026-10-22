import type { ReactNode } from 'react'
import { StackIcon } from '@phosphor-icons/react/dist/ssr'
import { PageBanner } from '../PageBanner'
import { LazyImagesDiagram } from '../banners/LazyPass3Diagrams'

/** ImagesBanner is the Images page header: hero while the node has no images, compact otherwise. */
export function ImagesBanner({
  empty,
  action,
}: Readonly<{ empty: boolean; action?: ReactNode }>) {
  const size = empty ? 'hero' : 'compact'
  return (
    <PageBanner
      icon={<StackIcon />}
      titleKey="images:title"
      descriptionKey={empty ? 'images:banner.description' : 'images:subtitle'}
      action={action}
      illustration={<LazyImagesDiagram size={size} />}
      size={size}
    />
  )
}
