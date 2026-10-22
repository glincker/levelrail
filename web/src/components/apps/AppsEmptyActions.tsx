import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  DownloadSimpleIcon,
  GitBranchIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'

// Secondary first-run actions that sit next to "New app" in the hero banner.
export function AppsEmptyActions() {
  const { t } = useTranslation('common')
  return (
    <>
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/settings/import-platform" />}
        nativeButton={false}
      >
        <DownloadSimpleIcon />
        {t('appsList.import', { defaultValue: 'Import' })}
      </Button>
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/settings/github-app" />}
        nativeButton={false}
      >
        <GitBranchIcon />
        {t('appsList.connectGit', { defaultValue: 'Connect Git' })}
      </Button>
    </>
  )
}
