import * as React from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { FileSearchGlassIcon } from '@thesvg/ui-react'
import type { PaletteItem } from './commandPaletteData'

// Resources, tags and saved views come from the unified search service; only the explorer entry point is static.
export function useResourcePaletteItems(): PaletteItem[] {
  const { t } = useTranslation('resources')
  const navigate = useNavigate()
  return React.useMemo<PaletteItem[]>(
    () => [
      {
        key: 'action-resource-explorer',
        label: t('palette.open'),
        keywords: t('palette.openKeywords'),
        group: 'Actions',
        icon: <FileSearchGlassIcon size="1.2em" />,
        run: () => void navigate({ to: '/resources' }),
      },
    ],
    [navigate, t],
  )
}
