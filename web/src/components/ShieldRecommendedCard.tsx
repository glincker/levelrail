import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { TrustedProxyFix } from './TrustedProxyFix'
import { useRecommendedShield } from '../queries/shieldRecommended'

export function ShieldRecommendedCard({ appName }: { appName: string }) {
  const { t } = useTranslation('domains')
  const [blockBots, setBlockBots] = useState(false)
  const botsId = useId()
  const mutation = useRecommendedShield(appName)
  const preview = mutation.data?.dry_run ? mutation.data : null

  function run(dryRun: boolean) {
    mutation.mutate(
      { dryRun, blockBots },
      {
        onSuccess: (data) => {
          if (!dryRun && !data.blocked) {
            toast.add({
              title: t('shieldRecommended.applied', {
                count: data.applied.length,
              }),
              type: 'success',
            })
          }
        },
        onError: (e) => {
          toast.add({ title: e.message, type: 'error' })
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheckGlassIcon
            className="size-5 text-tone-success"
            aria-hidden="true"
          />
          {t('shieldRecommended.title')}
        </CardTitle>
        <CardDescription>{t('shieldRecommended.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <label htmlFor={botsId} className="flex items-center gap-2 text-sm">
          <Checkbox
            id={botsId}
            checked={blockBots}
            onCheckedChange={(checked) => {
              setBlockBots(checked === true)
            }}
          />
          {t('shieldRecommended.blockBots')}
        </label>
        {mutation.data?.blocked ? (
          <>
            <Alert variant="destructive">
              <AlertDescription>
                {t('shieldRecommended.blockedProxy')}
              </AlertDescription>
            </Alert>
            <TrustedProxyFix />
          </>
        ) : null}
        {preview && !preview.blocked ? (
          <div className="space-y-1 text-sm" role="status">
            <p>
              {t('shieldRecommended.preview', {
                rate: preview.rate_per_minute,
                concurrent: preview.max_concurrent,
                count: preview.applied.length,
              })}
            </p>
            {preview.skipped.length > 0 ? (
              <p className="text-muted-foreground">
                {t('shieldRecommended.skipped', {
                  count: preview.skipped.length,
                })}
              </p>
            ) : null}
          </div>
        ) : null}
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={mutation.isPending}
            onClick={() => {
              run(true)
            }}
          >
            {t('shieldRecommended.preview_button')}
          </Button>
          <Button
            type="button"
            size="sm"
            disabled={mutation.isPending}
            onClick={() => {
              run(false)
            }}
          >
            {t('shieldRecommended.apply')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
