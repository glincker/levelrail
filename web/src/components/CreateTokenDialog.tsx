import { useMemo, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { z } from 'zod'
import { KeyIcon } from '@phosphor-icons/react/dist/ssr'
import { Dialog, DialogContent, DialogTrigger } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { AbilitiesField } from './AbilitiesField'
import { TokenCreatedView } from './TokenCreatedView'
import { InfoTip } from './kit'
import { CreateFlowShell } from './CreateFlowKit'
import { TOKEN_STEPS } from './tokenCreateSteps'
import { useCreateToken } from '../queries/tokens'
import { SIGN_IN_APPROVE } from '../types/token'
import type { CreateTokenResponse, TokenAbility } from '../types/token'

// 'never' sends no expires_in_days at all, matching the Go side's omitempty.
const EXPIRATION_OPTIONS = [
  { value: 'never', days: undefined },
  { value: '1', days: 1 },
  { value: '7', days: 7 },
  { value: '30', days: 30 },
  { value: '90', days: 90 },
  { value: '365', days: 365 },
] as const satisfies { value: string; days: number | undefined }[]

function createTokenSchema(t: TFunction<'settings'>) {
  return z
    .object({
      name: z.string().trim().min(1, t('tokens.create.nameRequired')),
      expiration: z.enum(['never', '1', '7', '30', '90', '365']),
      agentName: z.string().trim().max(64, t('tokens.create.agentTooLong')),
      abilities: z.array(
        z.enum([
          'read',
          'read:sensitive',
          'write',
          'write:sensitive',
          'deploy',
          'root',
        ]),
      ),
      signInApprove: z.boolean(),
    })
    .refine((v) => v.abilities.length > 0 || v.signInApprove, {
      message: t('tokens.create.abilityRequired'),
      path: ['abilities'],
    })
    .refine((v) => !(v.signInApprove && v.abilities.includes('root')), {
      message: t('tokens.create.rootExclusive'),
      path: ['signInApprove'],
    })
}

type CreateTokenFormValues = z.infer<ReturnType<typeof createTokenSchema>>

export function CreateTokenDialog() {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState<CreateTokenResponse | null>(null)
  const createToken = useCreateToken()
  const schema = useMemo(() => createTokenSchema(t), [t])
  const { control, register, handleSubmit, formState, reset } =
    useForm<CreateTokenFormValues>({
      resolver: zodResolver(schema),
      defaultValues: {
        name: '',
        expiration: 'never',
        agentName: '',
        abilities: [],
        signInApprove: false,
      },
    })

  const onSubmit = handleSubmit((values) => {
    const days = EXPIRATION_OPTIONS.find(
      (option) => option.value === values.expiration,
    )?.days
    const abilities: TokenAbility[] = values.signInApprove
      ? [...values.abilities, SIGN_IN_APPROVE]
      : values.abilities
    createToken.mutate(
      {
        name: values.name.trim(),
        abilities,
        ...(days ? { expires_in_days: days } : {}),
        ...(values.agentName.trim()
          ? { agent: { name: values.agentName.trim() } }
          : {}),
      },
      { onSuccess: setCreated },
    )
  })

  // Closing is the point of no return: the plaintext is never returned again.
  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setCreated(null)
      reset()
      createToken.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>
        {t('tokens.create.title')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {created ? (
          <TokenCreatedView
            created={created}
            onDone={() => {
              handleOpenChange(false)
            }}
          />
        ) : (
          <CreateFlowShell
            icon={<KeyIcon className="size-4 text-muted-foreground" />}
            title={t('tokens.create.title')}
            description={t('tokens.create.description')}
            steps={TOKEN_STEPS}
            currentStepIndex={0}
            onSubmit={(e) => {
              void onSubmit(e)
            }}
            error={createToken.isError ? createToken.error.message : null}
            submitLabel={t('tokens.create.submit')}
            submitPendingLabel={t('tokens.create.submitting')}
            pending={createToken.isPending}
          >
            <Field>
              <FieldLabel htmlFor="token-name">
                {t('tokens.create.name')}
              </FieldLabel>
              <Input
                id="token-name"
                placeholder={t('tokens.create.namePlaceholder')}
                {...register('name')}
              />
              <FieldError errors={[formState.errors.name]} />
            </Field>

            <Field>
              <FieldLabel htmlFor="token-expiration">
                {t('tokens.create.expiration')}
              </FieldLabel>
              <Controller
                control={control}
                name="expiration"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="token-expiration" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {EXPIRATION_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.days === undefined
                            ? t('tokens.create.never')
                            : t('tokens.create.days', { count: option.days })}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </Field>

            <Field>
              <div className="flex items-center gap-1">
                <FieldLabel htmlFor="token-agent">
                  {t('tokens.create.agent')}
                </FieldLabel>
                <InfoTip label={t('tokens.create.agentTipLabel')}>
                  {t('tokens.create.agentTip')}
                </InfoTip>
              </div>
              <Input
                id="token-agent"
                placeholder={t('tokens.create.agentPlaceholder')}
                {...register('agentName')}
              />
              <FieldError errors={[formState.errors.agentName]} />
            </Field>

            <Controller
              control={control}
              name="abilities"
              render={({ field }) => (
                <AbilitiesField
                  value={field.value}
                  onChange={field.onChange}
                  error={formState.errors.abilities}
                />
              )}
            />

            <Controller
              control={control}
              name="signInApprove"
              render={({ field }) => (
                <Field>
                  <FieldLabel>
                    <Field orientation="horizontal">
                      <Checkbox
                        checked={field.value}
                        onCheckedChange={(checked) => {
                          field.onChange(checked === true)
                        }}
                      />
                      <FieldContent>
                        <FieldTitle>
                          {t('tokens.create.signInApprove')}
                        </FieldTitle>
                        <FieldDescription className="text-xs">
                          {t('tokens.create.signInApproveHint')}
                        </FieldDescription>
                      </FieldContent>
                    </Field>
                  </FieldLabel>
                  <FieldError errors={[formState.errors.signInApprove]} />
                </Field>
              )}
            />
          </CreateFlowShell>
        )}
      </DialogContent>
    </Dialog>
  )
}
