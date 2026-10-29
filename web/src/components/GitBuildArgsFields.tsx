import { useTranslation } from 'react-i18next'
import {
  useFieldArray,
  type Control,
  type FormState,
  type UseFormRegister,
} from 'react-hook-form'
import { PlusGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { FormInput, FormOutput } from './CreateAppFromGitFields'

// Same add/remove-row pattern LabelsEditor.tsx uses for Docker labels,
// bound to the create-app form's own control/register.
export function BuildArgsFields({
  control,
  register,
  formState,
  disabled,
}: {
  control: Control<FormInput, unknown, FormOutput>
  register: UseFormRegister<FormInput>
  formState: FormState<FormInput>
  disabled: boolean
}) {
  const { t } = useTranslation('gitBuild')
  const { fields, append, remove } = useFieldArray({
    control,
    name: 'buildArgs',
  })

  return (
    <Field>
      <FieldLabel>{t('sourceFields.buildArgs')}</FieldLabel>
      <FieldDescription>{t('sourceFields.buildArgsHelp')}</FieldDescription>
      <div className="flex flex-col gap-2">
        {fields.map((field, index) => (
          <div key={field.id} className="flex items-start gap-2">
            <div className="flex-1">
              <Input
                {...register(`buildArgs.${index}.key`)}
                className="font-mono"
                placeholder="KEY"
                autoComplete="off"
                spellCheck={false}
                disabled={disabled}
                aria-label={t('sourceFields.buildArgKey')}
              />
              <FieldError
                errors={
                  formState.errors.buildArgs?.[index]?.key
                    ? [formState.errors.buildArgs[index]?.key]
                    : undefined
                }
              />
            </div>
            <Input
              {...register(`buildArgs.${index}.value`)}
              className="flex-1 font-mono"
              placeholder={t('sourceFields.buildArgValuePlaceholder')}
              autoComplete="off"
              spellCheck={false}
              disabled={disabled}
              aria-label={t('sourceFields.buildArgValue')}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              disabled={disabled}
              onClick={() => {
                remove(index)
              }}
            >
              <XGlassIcon size="1.4em" />
              <span className="sr-only">
                {t('sourceFields.removeBuildArg')}
              </span>
            </Button>
          </div>
        ))}
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="w-fit"
        disabled={disabled}
        onClick={() => {
          append({ key: '', value: '' })
        }}
      >
        <PlusGlassIcon size="1.4em" />
        {t('sourceFields.addBuildArg')}
      </Button>
    </Field>
  )
}
