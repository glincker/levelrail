import { useTranslation } from 'react-i18next'
import { LifebuoyIcon } from '@phosphor-icons/react/dist/ssr'
import type { SSOSettings } from '@/queries/sso'
import { useSetSSOEnforce } from '@/queries/sso'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Field, FieldLabel } from '@/components/ui/field'
import { FormSection } from '@/components/ui/form-section'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'

/** SSOEnforcementCard toggles "SSO required"; root keeps the password path. */
export function SSOEnforcementCard({ settings }: { settings: SSOSettings }) {
  const { t } = useTranslation('sso')
  const enforce = useSetSSOEnforce()
  return (
    <FormSection
      title={t('enforce.title')}
      description={t('enforce.description')}
      error={enforce.isError ? enforce.error.message : undefined}
    >
      <Field>
        <div className="flex items-center justify-between gap-3">
          <FieldLabel htmlFor="sso-enforce" className="font-normal">
            {t('enforce.label')}
          </FieldLabel>
          <Switch
            id="sso-enforce"
            checked={settings.enforce_sso}
            disabled={enforce.isPending}
            onCheckedChange={(on) =>
              enforce.mutate(on, {
                onSuccess: () =>
                  toast.add({
                    title: on ? t('enforce.on') : t('enforce.off'),
                    type: 'success',
                  }),
              })
            }
          />
        </div>
      </Field>
      <Alert>
        <LifebuoyIcon size="1.4em" />
        <AlertDescription>{t('enforce.breakGlass')}</AlertDescription>
      </Alert>
    </FormSection>
  )
}
