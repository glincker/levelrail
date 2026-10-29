import * as React from 'react'
import type { TFunction } from 'i18next'
import {
  BinaryTreeGlassIcon,
  BookOpenGlassIcon,
  BookmarkGlassIcon,
  DatabaseGlassIcon,
  FolderGlassIcon,
  GlobeGlassIcon,
  LayersGlassIcon,
  LayoutGridGlassIcon,
  RocketGlassIcon,
  ServerGlassIcon,
  SettingsGlassIcon,
  TagGlassIcon,
  ZapGlassIcon,
} from '@thesvg/ui-react'
import type { SearchHit } from '../queries/search'
import type { PaletteItem } from './commandPaletteData'

const KIND_ICONS: Record<string, React.ReactNode> = {
  app: <LayersGlassIcon size="1.2em" />,
  database: <DatabaseGlassIcon size="1.2em" />,
  node: <ServerGlassIcon size="1.2em" />,
  domain: <GlobeGlassIcon size="1.2em" />,
  template: <LayoutGridGlassIcon size="1.2em" />,
  project: <FolderGlassIcon size="1.2em" />,
  environment: <BinaryTreeGlassIcon size="1.2em" />,
  tag: <TagGlassIcon size="1.2em" />,
  deployment: <RocketGlassIcon size="1.2em" />,
  setting: <SettingsGlassIcon size="1.2em" />,
  doc: <BookOpenGlassIcon size="1.2em" />,
  action: <ZapGlassIcon size="1.2em" />,
  saved: <BookmarkGlassIcon size="1.2em" />,
}

export function kindIcon(kind: string): React.ReactNode {
  return KIND_ICONS[kind] ?? <LayersGlassIcon size="1.2em" />
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
    highlightSpans: hit.title_spans ?? [],
    searchRef: hit.ref,
    preview: {
      kind:
        hit.kind === 'app' || hit.kind === 'template' || hit.kind === 'action'
          ? hit.kind
          : 'page',
      description: hit.subtitle,
      path: hit.href?.split('?')[0],
      primaryLabel: hit.kind === 'template' ? t('preview.deploy') : undefined,
    },
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
