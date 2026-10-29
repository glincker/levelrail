import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { DownloadGlassIcon, GitBranchGlassIcon } from '@thesvg/ui-react'
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
        <DownloadGlassIcon size="1.4em" />
        {t('appsList.import', { defaultValue: 'Import' })}
      </Button>
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/settings/github-app" />}
        nativeButton={false}
      >
        <GitBranchGlassIcon size="1.4em" />
        {t('appsList.connectGit', { defaultValue: 'Connect Git' })}
      </Button>
    </>
  )
}
