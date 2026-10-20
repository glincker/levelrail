import { useTranslation } from 'react-i18next'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type {
  DeployFieldError,
  DeployFormState,
} from '../../lib/modelDeployForm'

type Errors = Partial<Record<DeployFieldError, true>>

function FieldProblem({ id, text }: { id: string; text: string }) {
  return (
    <p id={id} role="alert" className="text-xs text-destructive">
      {text}
    </p>
  )
}

// GPUs, context, residency, swap group, domain and engine extras, all with
// defaults, shown inside the collapsed Advanced section.
export function DeployAdvancedFields({
  form,
  errors,
  set,
}: {
  form: DeployFormState
  errors: Errors
  set: <K extends keyof DeployFormState>(
    key: K,
    value: DeployFormState[K],
  ) => void
}) {
  const { t } = useTranslation('models')
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Field>
        <FieldLabel htmlFor="model-gpus">{t('deploy.fields.gpus')}</FieldLabel>
        <Input
          id="model-gpus"
          value={form.gpus}
          onChange={(e) => {
            set('gpus', e.target.value)
          }}
          placeholder={t('deploy.fields.gpusPlaceholder')}
          aria-invalid={errors.gpus === true}
          aria-describedby="model-gpus-hint"
        />
        {errors.gpus ? (
          <FieldProblem id="model-gpus-hint" text={t('deploy.errors.gpus')} />
        ) : (
          <FieldHint id="model-gpus-hint">
            {t('deploy.fields.gpusHint')}
          </FieldHint>
        )}
      </Field>
      <Field>
        <FieldLabel htmlFor="model-context">
          {t('deploy.fields.context')}
        </FieldLabel>
        <Input
          id="model-context"
          value={form.context}
          onChange={(e) => {
            set('context', e.target.value)
          }}
          placeholder={t('deploy.fields.contextPlaceholder')}
          inputMode="numeric"
          aria-invalid={errors.context === true}
          aria-describedby="model-context-hint"
        />
        {errors.context ? (
          <FieldProblem
            id="model-context-hint"
            text={t('deploy.errors.context')}
          />
        ) : (
          <FieldHint id="model-context-hint">
            {t('deploy.fields.contextHint')}
          </FieldHint>
        )}
      </Field>
      {form.engine === 'vllm' ? (
        <Field className="sm:col-span-2">
          <FieldLabel htmlFor="model-quant">
            {t('deploy.fields.quantization')}
          </FieldLabel>
          <Input
            id="model-quant"
            value={form.quantization}
            onChange={(e) => {
              set('quantization', e.target.value)
            }}
          />
          <FieldHint>{t('deploy.fields.quantizationHint')}</FieldHint>
        </Field>
      ) : null}
      <Field>
        <FieldLabel htmlFor="model-residency">
          {t('deploy.fields.residency')}
        </FieldLabel>
        <Select
          value={form.residency}
          onValueChange={(v) => {
            if (v === 'always' || v === 'on_demand') set('residency', v)
          }}
        >
          <SelectTrigger id="model-residency" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="always">
              {t('deploy.fields.residencyAlways')}
            </SelectItem>
            <SelectItem value="on_demand">
              {t('deploy.fields.residencyOnDemand')}
            </SelectItem>
          </SelectContent>
        </Select>
        <FieldHint>{t('deploy.fields.residencyHint')}</FieldHint>
      </Field>
      {form.residency === 'on_demand' ? (
        <Field>
          <FieldLabel htmlFor="model-idle">
            {t('deploy.fields.idleMinutes')}
          </FieldLabel>
          <Input
            id="model-idle"
            value={form.idleMinutes}
            onChange={(e) => {
              set('idleMinutes', e.target.value)
            }}
            placeholder={t('deploy.fields.idlePlaceholder')}
            inputMode="numeric"
            aria-invalid={errors.idle === true}
            aria-describedby="model-idle-hint"
          />
          {errors.idle ? (
            <FieldProblem id="model-idle-hint" text={t('deploy.errors.idle')} />
          ) : (
            <FieldHint id="model-idle-hint">
              {t('deploy.fields.idleMinutesHint')}
            </FieldHint>
          )}
        </Field>
      ) : null}
      <Field>
        <FieldLabel htmlFor="model-swap-group">
          {t('deploy.fields.swapGroup')}
        </FieldLabel>
        <Input
          id="model-swap-group"
          value={form.swapGroup}
          onChange={(e) => {
            set('swapGroup', e.target.value)
          }}
          placeholder={t('deploy.fields.swapGroupPlaceholder')}
          autoComplete="off"
          aria-invalid={errors.swapGroup === true}
          aria-describedby="model-swap-hint"
        />
        {errors.swapGroup ? (
          <FieldProblem
            id="model-swap-hint"
            text={t('deploy.errors.swapGroup')}
          />
        ) : (
          <FieldHint id="model-swap-hint">
            {t('deploy.fields.swapGroupHint')}
          </FieldHint>
        )}
      </Field>
      <Field>
        <FieldLabel htmlFor="model-domain">
          {t('deploy.fields.domain')}
        </FieldLabel>
        <Input
          id="model-domain"
          value={form.domain}
          onChange={(e) => {
            set('domain', e.target.value)
          }}
          placeholder={t('deploy.fields.domainPlaceholder')}
          autoComplete="off"
        />
        <FieldHint>{t('deploy.fields.domainHint')}</FieldHint>
      </Field>
      {form.engine !== 'ollama' ? (
        <Field className="sm:col-span-2">
          <FieldLabel htmlFor="model-hf-token">
            {t('deploy.fields.hfToken')}
          </FieldLabel>
          <Input
            id="model-hf-token"
            type="password"
            value={form.hfToken}
            onChange={(e) => {
              set('hfToken', e.target.value)
            }}
            placeholder="hf_..."
            autoComplete="off"
          />
          <FieldHint>{t('deploy.fields.hfTokenHint')}</FieldHint>
        </Field>
      ) : null}
    </div>
  )
}
