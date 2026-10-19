import * as React from 'react'
import type { TFunction } from 'i18next'
import {
  BookOpenIcon,
  BookmarkSimpleIcon,
  DatabaseIcon,
  FolderIcon,
  GearIcon,
  GlobeIcon,
  HardDrivesIcon,
  LightningIcon,
  RocketLaunchIcon,
  SquaresFourIcon,
  StackIcon,
  TagIcon,
  TreeStructureIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { SearchHit } from '../queries/search'
import type { PaletteItem } from './commandPaletteData'

const KIND_ICONS: Record<string, React.ReactNode> = {
  app: <StackIcon />,
  database: <DatabaseIcon />,
  node: <HardDrivesIcon />,
  domain: <GlobeIcon />,
  template: <SquaresFourIcon />,
  project: <FolderIcon />,
  environment: <TreeStructureIcon />,
  tag: <TagIcon />,
  deployment: <RocketLaunchIcon />,
  setting: <GearIcon />,
  doc: <BookOpenIcon />,
  action: <LightningIcon />,
  saved: <BookmarkSimpleIcon />,
}

export function kindIcon(kind: string): React.ReactNode {
  return KIND_ICONS[kind] ?? <StackIcon />
}

export type HrefNavigate = (opts: {
  to: string
  search?: Record<string, string>
}) => void

// Hit hrefs carry their query string ("/apps/x?action=restart"); the router wants it split.
export function navigateHref(navigate: HrefNavigate, href: string): void {
  const [path, query] = href.split('?')
  const to = path ?? href
  if (!query) {
    navigate({ to })
    return
  }
  navigate({ to, search: Object.fromEntries(new URLSearchParams(query)) })
}

// Last ref segment: "app:web" and "action:app.restart:web" both end in the resource name.
export function refName(ref: string): string {
  return ref.slice(ref.lastIndexOf(':') + 1)
}

export interface HitItemContext {
  terms: string[]
  t: TFunction<'palette'>
  navigate: HrefNavigate
  restartApp: (name: string) => void
  applySaved: (hit: SearchHit) => void
}

export function hitToItem(hit: SearchHit, ctx: HitItemContext): PaletteItem {
  const { t, navigate } = ctx
  const isAction = hit.kind === 'action' && hit.action
  const base: PaletteItem = {
    key: `hit-${hit.ref}`,
    label: hit.title,
    group: hit.kind,
    icon: kindIcon(hit.kind),
    subtitle: isAction ? hit.app_name : hit.subtitle,
    highlightTerms: ctx.terms,
    searchRef: hit.ref,
    run: () => {
      if (hit.href) navigateHref(navigate, hit.href)
    },
  }
  if (hit.kind === 'saved') {
    return {
      ...base,
      keepOpen: true,
      searchRef: undefined,
      run: () => ctx.applySaved(hit),
    }
  }
  if (isAction && hit.action) {
    const name = refName(hit.ref)
    return {
      ...base,
      confirm: hit.action.confirm,
      run:
        hit.action.id === 'app.restart' ? () => ctx.restartApp(name) : base.run,
      ...newTabSecondary(hit, t),
    }
  }
  if (hit.kind === 'app') {
    const name = refName(hit.ref)
    return {
      ...base,
      secondary: () =>
        navigate({ to: `/apps/${encodeURIComponent(name)}/logs` }),
      secondaryHint: t('hint.logs'),
    }
  }
  return { ...base, ...newTabSecondary(hit, t) }
}

function newTabSecondary(
  hit: SearchHit,
  t: TFunction<'palette'>,
): Pick<PaletteItem, 'secondary' | 'secondaryHint'> {
  const href = hit.href
  if (!href) return {}
  return {
    secondary: () => void window.open(href, '_blank', 'noopener,noreferrer'),
    secondaryHint: t('hint.newTab'),
  }
}
