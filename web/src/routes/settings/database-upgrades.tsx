import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { PageHeader } from '../../components/shell/PageHeader'
import { PolicyForm } from '../../components/databaseUpgrades/PolicyForm'
import {
  usePlatformUpgradePolicy,
  useSetPlatformUpgradePolicy,
} from '../../queries/databaseUpgrades'

export const Route = createFileRoute('/settings/database-upgrades')({
  component: DatabaseUpgradesSettingsPage,
})

function DatabaseUpgradesSettingsPage() {
  const { t } = useTranslation('settings')
  const { t: tUpgrades } = useTranslation('databaseUpgrades')
  const { data, isLoading, error } = usePlatformUpgradePolicy()
  const save = useSetPlatformUpgradePolicy()

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('databaseUpgrades.title')}
        description={t('databaseUpgrades.description')}
      />
      <Card>
        <CardHeader>
          <CardTitle>{t('databaseUpgrades.formTitle')}</CardTitle>
          <CardDescription>
            {t('databaseUpgrades.formDescription')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Skeleton className="h-48 w-full" />
          ) : error || !data ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t('databaseUpgrades.loadFailed', {
                  error: error?.message ?? '',
                })}
              </AlertDescription>
            </Alert>
          ) : (
            <PolicyForm
              key={JSON.stringify(data)}
              idPrefix="platform-upgrade-policy"
              policy={data}
              editable
              saving={save.isPending}
              onSave={(input) => {
                save.mutate(input, {
                  onSuccess: () => {
                    toast.add({
                      title: tUpgrades('policy.saved'),
                      type: 'success',
                    })
                  },
                  onError: (err) => {
                    toast.add({
                      title: tUpgrades('policy.failedToast'),
                      description: err.message,
                      type: 'error',
                    })
                  },
                })
              }}
            />
          )}
        </CardContent>
      </Card>
    </div>
  )
}
