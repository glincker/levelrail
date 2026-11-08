import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { TriangleAlertGlassIcon, UploadGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useImportCustomTemplate } from '../queries/customTemplates'
import {
  TEMPLATE_ID_RE,
  TEMPLATE_VAR_RE,
  importTemplateMaxBytes,
  parseVariables,
  slugifyTemplateId,
} from '../lib/importTemplate'

const schema = z.object({
  id: z.string().trim().regex(TEMPLATE_ID_RE, 'idInvalid'),
  name: z.string().trim().min(1, 'nameRequired'),
  description: z.string().trim().optional(),
  compose: z.string().trim().min(1, 'composeRequired'),
  variables: z
    .string()
    .refine((v) => parseVariables(v).every((n) => TEMPLATE_VAR_RE.test(n)), {
      message: 'variablesInvalid',
    }),
})
type FormValues = z.infer<typeof schema>

const EMPTY: FormValues = {
  id: '',
  name: '',
  description: '',
  compose: '',
  variables: '',
}

// Saves an operator-written compose file as a custom template. It never
// deploys anything; the template then shows under "Your templates".
export function ImportTemplateDialog() {
  const { t } = useTranslation('templates')
  const [open, setOpen] = useState(false)
  const [fileError, setFileError] = useState(false)
  const importTemplate = useImportCustomTemplate()
  const { register, handleSubmit, formState, reset, setValue } =
    useForm<FormValues>({
      resolver: zodResolver(schema),
      defaultValues: EMPTY,
    })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(EMPTY)
      importTemplate.reset()
      setFileError(false)
    }
  }

  async function onFile(file: File | undefined) {
    if (!file) return
    if (file.size > importTemplateMaxBytes()) {
      setFileError(true)
      return
    }
    setFileError(false)
    setValue('compose', await file.text(), { shouldValidate: true })
  }

  const onSubmit = handleSubmit((values) => {
    const variables = parseVariables(values.variables)
    importTemplate.mutate(
      {
        id: values.id.trim(),
        name: values.name.trim(),
        description: values.description?.trim() || undefined,
        compose_yaml: values.compose,
        variables: variables.length > 0 ? variables : undefined,
      },
      {
        onSuccess: () => {
          toast.add({
            title: t('importTemplate.success', { name: values.name.trim() }),
            type: 'success',
          })
          handleOpenChange(false)
        },
      },
    )
  })

  const err = (key: keyof FormValues) => {
    const m = formState.errors[key]?.message
    return m ? [{ message: t(`importTemplate.${m}`) }] : []
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <UploadGlassIcon className="size-5" aria-hidden="true" />
        {t('importTemplate.button')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('importTemplate.title')}</DialogTitle>
          <DialogDescription>
            {t('importTemplate.description')}
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-3"
        >
          <div className="flex gap-2">
            <Field>
              <FieldLabel htmlFor="import-template-name">
                {t('importTemplate.nameLabel')}
              </FieldLabel>
              <Input
                id="import-template-name"
                {...register('name', {
                  onChange: (e: React.ChangeEvent<HTMLInputElement>) => {
                    if (!formState.dirtyFields.id) {
                      setValue('id', slugifyTemplateId(e.target.value))
                    }
                  },
                })}
              />
              <FieldError errors={err('name')} />
            </Field>
            <Field>
              <FieldLabel htmlFor="import-template-id">
                {t('importTemplate.idLabel')}
              </FieldLabel>
              <Input
                id="import-template-id"
                className="font-mono"
                {...register('id')}
              />
              <FieldError errors={err('id')} />
            </Field>
          </div>
          <Field>
            <FieldLabel htmlFor="import-template-description">
              {t('importTemplate.descriptionLabel')}
            </FieldLabel>
            <Input
              id="import-template-description"
              {...register('description')}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="import-template-file">
              {t('importTemplate.fileLabel')}
            </FieldLabel>
            <Input
              id="import-template-file"
              type="file"
              accept=".yml,.yaml,text/yaml,text/plain"
              onChange={(e) => {
                void onFile(e.target.files?.[0])
              }}
            />
            {fileError ? (
              <p role="alert" className="text-sm text-destructive">
                {t('importTemplate.tooLarge')}
              </p>
            ) : null}
          </Field>
          <Field>
            <FieldLabel htmlFor="import-template-compose">
              {t('importTemplate.composeLabel')}
            </FieldLabel>
            <Textarea
              id="import-template-compose"
              rows={8}
              className="font-mono text-xs"
              placeholder={t('importTemplate.composePlaceholder')}
              {...register('compose')}
            />
            <FieldError errors={err('compose')} />
          </Field>
          <Field>
            <FieldLabel htmlFor="import-template-variables">
              {t('importTemplate.variablesLabel')}
            </FieldLabel>
            <Input
              id="import-template-variables"
              className="font-mono"
              placeholder={t('importTemplate.variablesPlaceholder')}
              {...register('variables')}
            />
            <FieldError errors={err('variables')} />
          </Field>

          {importTemplate.isError ? (
            <p
              role="alert"
              className="flex items-start gap-1.5 text-sm text-destructive"
            >
              <TriangleAlertGlassIcon
                className="mt-0.5 size-6 shrink-0 text-tone-warning"
                aria-hidden="true"
              />
              {importTemplate.error.message}
            </p>
          ) : null}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                handleOpenChange(false)
              }}
            >
              {t('importTemplate.cancel')}
            </Button>
            <Button type="submit" disabled={importTemplate.isPending}>
              {importTemplate.isPending
                ? t('importTemplate.submitting')
                : t('importTemplate.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
