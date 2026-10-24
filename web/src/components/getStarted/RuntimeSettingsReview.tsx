import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { SettingRows } from '@/components/ui/setting-row'
import { toast } from '@/components/ui/toast'
import { RuntimeSettingRow } from '@/components/settings/RuntimeSettingRow'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  useAckRuntimeOnboarding,
  useRuntimeOnboarding,
} from '../../queries/runtimeSettings'

/** Renders nothing until the review has items and is still unacknowledged. */
export function RuntimeSettingsReview() {
  const { t } = useTranslation('runtimeSettings')
  const isRoot = useIsRoot()
  const { data } = useRuntimeOnboarding()
  const ack = useAckRuntimeOnboarding()

  if (!data || data.reviewed || !data.items?.length) return null

  return (
    <section
      aria-labelledby="runtime-settings-review"
      className="space-y-3 rounded-lg border border-border bg-card p-4"
    >
      <div className="flex items-start gap-3">
        <ShieldCheckIcon
          className="mt-0.5 size-5 shrink-0 text-tone-success"
          aria-hidden="true"
        />
        <div className="min-w-0 space-y-1">
          <h2
            id="runtime-settings-review"
            className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground"
          >
            {t('onboarding.title')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('onboarding.intro')}
          </p>
        </div>
      </div>
      <SettingRows>
        {data.items.map((s) => (
          <RuntimeSettingRow
            key={s.key}
            setting={s}
            canEdit={isRoot}
            showDefault
          />
        ))}
      </SettingRows>
      <div className="flex flex-wrap items-center gap-2">
        {isRoot ? (
          <Button
            type="button"
            size="sm"
            disabled={ack.isPending}
            onClick={() => {
              ack.mutate(undefined, {
                onError: (err) => {
                  toast.add({
                    type: 'error',
                    title: t('onboarding.ackError', { message: err.message }),
                  })
                },
              })
            }}
          >
            {ack.isPending ? t('onboarding.acking') : t('onboarding.ack')}
          </Button>
        ) : null}
        <Button
          size="sm"
          variant="ghost"
          render={<Link to="/settings/security-runtime" />}
        >
          {t('onboarding.openSettings')}
        </Button>
      </div>
    </section>
  )
}
