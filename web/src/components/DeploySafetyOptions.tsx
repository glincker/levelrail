import { SnowflakeGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useDeployFreezeOptional } from '../queries/deployFreeze'
import type { DeploySafetyValues } from '../lib/imageDigest'

// DeploySafetyOptions holds a manual deploy's "pull fresh" choice and, only
// while the app is frozen, the required override reason.
export function DeploySafetyOptions({
  appName,
  values,
  onChange,
}: {
  appName: string
  values: DeploySafetyValues
  onChange: (next: DeploySafetyValues) => void
}) {
  const { t } = useTranslation('deploySafety')
  const freeze = useDeployFreezeOptional(appName)
  const frozen = freeze.data?.status.frozen === true
  const until = freeze.data?.status.until

  return (
    <div className="mt-3 space-y-3">
      <div className="flex items-start gap-2">
        <Checkbox
          id={`deploy-pull-${appName}`}
          checked={values.pull}
          onCheckedChange={(checked) =>
            onChange({ ...values, pull: checked === true })
          }
        />
        <Label
          htmlFor={`deploy-pull-${appName}`}
          className="text-sm font-normal text-muted-foreground"
        >
          {t('pullFresh')}
        </Label>
      </div>
      {frozen ? (
        <Alert variant="warning" role="status" className="p-3">
          <SnowflakeGlassIcon className="size-5" aria-hidden="true" />
          <AlertTitle>
            {until
              ? t('freezeBanner.titleUntil', {
                  when: new Date(until).toLocaleString(),
                })
              : t('freezeBanner.title')}
          </AlertTitle>
          <AlertDescription className="text-xs">
            {t('freezeBanner.body')}
          </AlertDescription>
          <Input
            className="col-span-full mt-2"
            aria-label={t('freezeBanner.reasonLabel')}
            placeholder={t('freezeBanner.reasonPlaceholder')}
            value={values.overrideReason}
            onChange={(e) =>
              onChange({ ...values, overrideReason: e.target.value })
            }
          />
        </Alert>
      ) : null}
    </div>
  )
}
