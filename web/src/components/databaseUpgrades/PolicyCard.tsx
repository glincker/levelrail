import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { useSetUpgradePolicy } from '../../queries/databaseUpgrades'
import type {
  DatabaseUpgrades,
  UpgradePolicyInput,
} from '../../types/databaseUpgrades'
import { PolicyForm } from './PolicyForm'
import { draftToInput, policyToDraft } from './upgradeHelpers'

function formatWhen(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

/** PolicyCard shows the effective policy and lets one database override or drop back to the platform default. */
export function PolicyCard({
  databaseName,
  data,
}: {
  databaseName: string
  data: DatabaseUpgrades
}) {
  const { t } = useTranslation('databaseUpgrades')
  const save = useSetUpgradePolicy(databaseName)
  const [overriding, setOverriding] = useState(false)
  const { policy } = data
  const inherited = policy.inherited && !overriding

  function submit(input: UpgradePolicyInput) {
    save.mutate(
      { ...input, inherit: false },
      {
        onSuccess: () => {
          setOverriding(false)
          toast.add({ title: t('policy.saved'), type: 'success' })
        },
        onError: (err) => {
          toast.add({
            title: t('policy.failedToast'),
            description: err.message,
            type: 'error',
          })
        },
      },
    )
  }

  function resetToDefault() {
    save.mutate(
      { ...draftToInput(policyToDraft(policy)), inherit: true },
      {
        onSuccess: () => {
          toast.add({ title: t('policy.saved'), type: 'success' })
        },
        onError: (err) => {
          toast.add({
            title: t('policy.failedToast'),
            description: err.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('policy.title')}</CardTitle>
        <CardDescription>
          {inherited ? t('policy.inheritedNote') : t('policy.description')}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid gap-2 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-muted-foreground">{t('policy.nextWindow')}</dt>
            <dd>
              {data.window_open
                ? t('policy.windowOpen')
                : data.next_window
                  ? formatWhen(data.next_window)
                  : '-'}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('policy.nextTarget')}</dt>
            <dd>
              {data.next_target
                ? data.next_target.version
                : t('policy.noNextTarget')}
            </dd>
          </div>
        </dl>
        <PolicyForm
          key={`${policy.auto_upgrade}|${policy.window_cron}|${policy.window_duration_seconds}|${policy.window_timezone}|${policy.verify_after}|${policy.revert_on_failure}|${policy.notify.join(',')}|${policy.inherited}|${overriding}`}
          idPrefix="db-upgrade-policy"
          policy={policy}
          editable={!inherited}
          saving={save.isPending}
          onSave={submit}
          extraActions={
            inherited ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  setOverriding(true)
                }}
              >
                {t('policy.override')}
              </Button>
            ) : !policy.inherited ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={save.isPending}
                onClick={resetToDefault}
              >
                {t('policy.useDefault')}
              </Button>
            ) : null
          }
        />
      </CardContent>
    </Card>
  )
}
