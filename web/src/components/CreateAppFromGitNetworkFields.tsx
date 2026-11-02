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
  return (
    <>
      <div className="space-y-4">
        <FieldSectionLabel>{/* i18n-ignore */}Networking</FieldSectionLabel>
        <Field>
          <FieldLabel htmlFor="git-app-port">
            {/* i18n-ignore */}Port
          </FieldLabel>
          <Input
            id="git-app-port"
            type="number"
            step="1"
            min="1"
            placeholder="e.g. 3000" // i18n-ignore
            disabled={locked}
            {...register('port')}
          />
          <FieldHint>
            The port your app listens on inside its container, e.g. 3000 for a
            typical Next.js app.
          </FieldHint>
          <FieldError errors={[formState.errors.port]} />
        </Field>

        <Field>
          <FieldLabel htmlFor="git-app-domain">
            {/* i18n-ignore */}Domain (optional)
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
          <FieldDescription>
            Routed once its DNS record points here. A TLS certificate is issued
            automatically, or add this later from the app&apos;s Domains tab.
          </FieldDescription>
        </Field>
      </div>

      <div className="space-y-4">
        <FieldSectionLabel>{/* i18n-ignore */}Health check</FieldSectionLabel>
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
            Enable a health check
          </FieldLabel>
        </Field>
        {healthCheckEnabled ? (
          <Field>
            <FieldLabel htmlFor="git-app-health-path">
              {/* i18n-ignore */}Path
            </FieldLabel>
            <Input
              id="git-app-health-path"
              className="font-mono"
              placeholder={HEALTH_CHECK_DEFAULT_PATH}
              disabled={locked}
              {...register('healthCheckPath')}
            />
            <FieldHint>
              Checked before cutting traffic to a new container, and to detect a
              crashed one afterward. Railpack-detected apps that don&rsquo;t
              serve this path should turn it off.
            </FieldHint>
            <FieldError errors={[formState.errors.healthCheckPath]} />
          </Field>
        ) : null}
      </div>
    </>
  )
}
