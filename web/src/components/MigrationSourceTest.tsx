import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, CircleXGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import type { AppImportSource } from '../queries/appImport'
import { useMigrationSourceTest } from '../queries/migrationJob'

// Reads the source once and stores nothing, so a wrong URL or token shows up before a job exists.
export function MigrationSourceTest({
  name,
  build,
  disabled,
}: {
  name: string
  build: () => AppImportSource
  disabled?: boolean
}) {
  const { t } = useTranslation('migration')
  const test = useMigrationSourceTest()
  const res = test.data
  return (
    <div className="space-y-2">
      <Button
        type="button"
        variant="outline"
        disabled={disabled || test.isPending}
        onClick={() => test.mutate(build())}
      >
        {test.isPending
          ? t('appImport.connect.testing')
          : t('appImport.connect.test')}
      </Button>
      <div aria-live="polite">
        {test.isError ? (
          <Alert variant="destructive">
            <AlertDescription className="break-words">
              {test.error.message}
            </AlertDescription>
          </Alert>
        ) : null}
        {res?.ok ? (
          <Alert>
            <AlertDescription className="space-y-1">
              <span className="flex items-start gap-2">
                <CircleCheckGlassIcon
                  className="mt-0.5 size-4.5 shrink-0 text-tone-success"
                  aria-hidden
                />
                {t('appImport.connect.testOk', {
                  name,
                  apps: res.apps,
                  databases: res.databases,
                  volumes: res.volumes,
                  domains: res.domains,
                  crons: res.crons,
                  unsupported: res.unsupported,
                })}
              </span>
              {res.servers.length > 0 ? (
                <span className="block text-xs text-muted-foreground">
                  {t('appImport.connect.testServers', {
                    list: res.servers.join(', '),
                  })}
                </span>
              ) : null}
              {res.warnings.map((w) => (
                <span key={w} className="block text-xs text-muted-foreground">
                  {w}
                </span>
              ))}
            </AlertDescription>
          </Alert>
        ) : null}
        {res && !res.ok ? (
          <Alert variant="destructive">
            <AlertDescription className="space-y-1 break-words">
              <span className="flex items-start gap-2">
                <CircleXGlassIcon
                  className="mt-0.5 size-4.5 shrink-0 text-tone-danger"
                  aria-hidden
                />
                {t(
                  `appImport.connect.testFailure.${res.failure ?? 'unreachable'}`,
                )}
              </span>
              {res.message ? (
                <span className="block text-xs">{res.message}</span>
              ) : null}
            </AlertDescription>
          </Alert>
        ) : null}
      </div>
    </div>
  )
}
