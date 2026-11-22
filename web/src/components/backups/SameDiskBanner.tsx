import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { buttonVariants } from '@/components/ui/button'
import type { CoverageTarget } from '../../types/backupCoverage'
import { allTargetsOnSameDisk } from './coverageHelpers'

/** SameDiskBanner nudges toward off-server storage when no backup would survive this server. */
export function SameDiskBanner({ targets }: { targets: CoverageTarget[] }) {
  const { t } = useTranslation('backups')
  if (!allTargetsOnSameDisk(targets)) return null
  return (
    <Alert>
      <TriangleAlertGlassIcon
        className="text-tone-warning"
        size="1.7em"
        aria-hidden="true"
      />
      <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
        <span>
          <strong>{t('sameDisk.title')}</strong> {t('sameDisk.body')}
        </span>
        <Link
          to="/settings/backup-targets"
          className={buttonVariants({ size: 'sm', variant: 'outline' })}
        >
          {t('sameDisk.connect')}
        </Link>
      </AlertDescription>
    </Alert>
  )
}
