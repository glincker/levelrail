import { useTranslation } from 'react-i18next'
import type {
  FieldErrors,
  UseFormRegister,
  UseFormSetValue,
} from 'react-hook-form'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldHint,
  FieldLabel,
  FieldSectionLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { HEALTH_CHECK_DEFAULT_PATH } from '../lib/healthCheckDefaults'
import type { FormInput } from '../lib/createAppFromGit'

// Port, domain and health check sections of the create-from-git form.
export function CreateAppFromGitNetworkFields({
  register,
  formState,
  setValue,
  healthCheckEnabled,
  locked,
}: {
  register: UseFormRegister<FormInput>
  formState: { errors: FieldErrors<FormInput> }
  setValue: UseFormSetValue<FormInput>
  healthCheckEnabled: boolean
  locked: boolean
}) {
  const { t } = useTranslation('gitSource')
  return (
    <>
      <div className="space-y-4">
        <FieldSectionLabel>{t('network.networking')}</FieldSectionLabel>
        <Field>
          <FieldLabel htmlFor="git-app-port">{t('network.port')}</FieldLabel>
          <Input
            id="git-app-port"
            type="number"
            step="1"
            min="1"
            placeholder={t('network.portPlaceholder')}
            disabled={locked}
            {...register('port')}
          />
          <FieldHint>{t('network.portHint')}</FieldHint>
          <FieldError errors={[formState.errors.port]} />
        </Field>

        <Field>
          <FieldLabel htmlFor="git-app-domain">
            {t('network.domain')}
          </FieldLabel>
          <Input
            id="git-app-domain"
            className="font-mono"
            placeholder="app.example.com" // i18n-ignore
            autoComplete="off"
            spellCheck={false}
            disabled={locked}
            {...register('domain')}
          />
          <FieldError errors={[formState.errors.domain]} />
          <FieldDescription>{t('network.domainDescription')}</FieldDescription>
        </Field>
      </div>

      <div className="space-y-4">
        <FieldSectionLabel>{t('network.healthCheck')}</FieldSectionLabel>
        <Field orientation="horizontal">
          <Switch
            id="git-app-health-enabled"
            checked={healthCheckEnabled}
            onCheckedChange={(checked) => {
              setValue('healthCheckEnabled', checked, { shouldValidate: true })
            }}
            disabled={locked}
          />
          <FieldLabel htmlFor="git-app-health-enabled">
            {t('network.healthEnable')}
          </FieldLabel>
        </Field>
        {healthCheckEnabled ? (
          <Field>
            <FieldLabel htmlFor="git-app-health-path">
              {t('network.healthPath')}
            </FieldLabel>
            <Input
              id="git-app-health-path"
              className="font-mono"
              placeholder={HEALTH_CHECK_DEFAULT_PATH}
              disabled={locked}
              {...register('healthCheckPath')}
            />
            <FieldHint>{t('network.healthHint')}</FieldHint>
            <FieldError errors={[formState.errors.healthCheckPath]} />
          </Field>
        ) : null}
      </div>
    </>
  )
}
