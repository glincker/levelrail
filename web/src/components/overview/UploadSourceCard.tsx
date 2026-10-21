import { useTranslation } from 'react-i18next'
import { UploadSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useBrand } from '../../hooks/useBrand'
import { cliCommand } from '../../lib/cliCommand'
import type { DeployAttempt } from '../../types/deployAttempt'
import { CliCommandBox } from '../newapp/CliCommandBox'
import { uploadFileCount } from './uploadSource'

/** Overview card for an app built from a directory uploaded by the CLI. */
export function UploadSourceCard({
  appName,
  attempt,
  isStatic = false,
}: {
  appName: string
  attempt: DeployAttempt
  isStatic?: boolean
}) {
  const { t } = useTranslation('newApp')
  const brand = useBrand()
  const files = uploadFileCount(attempt.commit_message)
  const hash = attempt.commit_sha?.slice(0, 12) ?? ''
  const command = cliCommand(
    brand.BinaryName,
    `deploy --app ${appName}${isStatic ? ' --static' : ''} --api-url ${window.location.origin}`,
  )

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <UploadSimpleIcon className="size-4" aria-hidden="true" />
          {t('uploadSource.title')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          <dt className="text-muted-foreground">{t('uploadSource.hash')}</dt>
          <dd className="font-mono">{hash}</dd>
          {files !== null ? (
            <>
              <dt className="text-muted-foreground">
                {t('uploadSource.files')}
              </dt>
              <dd>{files}</dd>
            </>
          ) : null}
        </dl>
        <p className="text-xs text-muted-foreground">
          {t('uploadSource.redeploy')}
        </p>
        <CliCommandBox command={command} />
      </CardContent>
    </Card>
  )
}
