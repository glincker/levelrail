import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import {
  useDeployUrlCopies,
  useUpdateDeployUrlCopies,
  type DeployUrlCopies,
  type DeployUrlCopiesMode,
} from '../queries/deployUrlCopies'
import { SettingsFormAlerts } from './SettingsCard'

export function DeployUrlCopiesCard() {
  const { data } = useDeployUrlCopies()
  if (!data) return null
  return <DeployUrlCopiesForm settings={data} />
}

function DeployUrlCopiesForm({ settings }: { settings: DeployUrlCopies }) {
  const { t } = useTranslation('settings')
  const update = useUpdateDeployUrlCopies()
  const [confirmPublic, setConfirmPublic] = useState(false)

  function apply(next: DeployUrlCopiesMode | '') {
    update.mutate(next, {
      onSuccess: () => {
        setConfirmPublic(false)
        toast.add({ title: t('deployUrlCopies.saved'), type: 'success' })
      },
    })
  }

  function choose(next: DeployUrlCopiesMode) {
    if (next === settings.mode) return
    if (next === 'public') {
      setConfirmPublic(true)
      return
    }
    apply(next)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GlobeGlassIcon className="size-5" aria-hidden="true" />
          {t('deployUrlCopies.title')}
          <Badge variant="outline">
            {t(`deployUrlCopies.source.${settings.source}`)}
          </Badge>
        </CardTitle>
        <CardDescription>{t('deployUrlCopies.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div
          role="radiogroup"
          aria-label={t('deployUrlCopies.modeLabel')}
          className="grid gap-1"
        >
          {settings.modes.map((m) => (
            <label
              key={m}
              className={cn(
                'flex cursor-pointer items-start gap-3 rounded-md px-3 py-2 text-sm',
                settings.mode === m ? 'bg-muted' : 'hover:bg-muted/50',
              )}
            >
              <input
                type="radio"
                name="deploy-url-copies-mode"
                value={m}
                checked={settings.mode === m}
                disabled={update.isPending}
                onChange={() => choose(m)}
                className="mt-1"
              />
              <span className="space-y-0.5">
                <span className="block font-medium">
                  {t(`deployUrlCopies.modes.${m}.label`)}
                </span>
                <span className="block text-muted-foreground">
                  {t(`deployUrlCopies.modes.${m}.description`)}
                </span>
              </span>
            </label>
          ))}
        </div>

        <SettingsFormAlerts
          alerts={[
            update.isError && { key: 'update', message: update.error.message },
          ]}
        />

        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="outline"
            onClick={() => apply('')}
            disabled={settings.source !== 'stored' || update.isPending}
          >
            {t('deployUrlCopies.reset', {
              mode: t(`deployUrlCopies.modes.${settings.default}.label`),
            })}
          </Button>
        </div>
      </CardContent>
      <ConfirmDialog
        open={confirmPublic}
        onOpenChange={setConfirmPublic}
        title={t('deployUrlCopies.confirm.title')}
        description={t('deployUrlCopies.confirm.description')}
        confirmLabel={t('deployUrlCopies.confirm.confirm')}
        pendingLabel={t('deployUrlCopies.saving')}
        pending={update.isPending}
        error={update.error?.message ?? null}
        onConfirm={() => apply('public')}
      />
    </Card>
  )
}
