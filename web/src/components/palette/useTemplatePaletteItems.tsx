import * as React from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { SquaresFourIcon } from '@phosphor-icons/react/dist/ssr'
import { rankItems } from '@/lib/paletteSearch'
import {
  serviceTemplatesQueryOptions,
  type ServiceTemplateListItem,
} from '../../queries/serviceTemplates'
import type { PaletteItem } from '../commandPaletteData'

const MAX_TEMPLATES = 4
const NO_HINTS: readonly string[] = []

function matchesHint(t: ServiceTemplateListItem, hints: readonly string[]) {
  const id = t.id.toLowerCase()
  const name = t.name.toLowerCase()
  return hints.some((h) => id.includes(h) || name.includes(h))
}

// Templates come from the existing catalog query; it only runs once the user types.
export function useTemplatePaletteItems(
  query: string,
  enabled: boolean,
  hints: readonly string[] = NO_HINTS,
): PaletteItem[] {
  const { t } = useTranslation('palette')
  const navigate = useNavigate()
  const templates = useQuery({
    ...serviceTemplatesQueryOptions(),
    enabled,
    retry: false,
  })
  const data = templates.data

  return React.useMemo<PaletteItem[]>(() => {
    if (!data || query.trim() === '') return []
    const toItem = (tpl: ServiceTemplateListItem): PaletteItem => ({
      key: `template-${tpl.id}`,
      label: tpl.name,
      subtitle: tpl.slogan,
      group: 'Templates',
      icon: <SquaresFourIcon />,
      keywords: `${tpl.category} ${tpl.id} ${tpl.slogan}`,
      run: () => void navigate({ to: '/deploy', search: { template: tpl.id } }),
      preview: {
        kind: 'template',
        description: tpl.slogan,
        primaryLabel: t('preview.deploy'),
        facts: [
          tpl.category,
          ...(tpl.needs_database ? [t('preview.needsDatabase')] : []),
          ...(tpl.has_volume ? [t('preview.hasVolume')] : []),
        ].filter((f) => f !== ''),
      },
    })
    const ranked = rankItems(data.map(toItem), query, (i) => i.keywords)
    const hinted = hints.length
      ? data.filter((tpl) => matchesHint(tpl, hints)).map(toItem)
      : []
    const seen = new Set<string>()
    return [...hinted.slice(0, 1), ...ranked]
      .filter((i) => !seen.has(i.key) && seen.add(i.key))
      .slice(0, MAX_TEMPLATES)
  }, [data, query, hints, navigate, t])
}
