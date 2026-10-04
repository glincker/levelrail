import { useState } from 'react'
import type { DialogControl } from './dialogControl'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  CheckCircleIcon,
  PackageIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useSaveAppAsTemplate } from '../queries/customTemplates'
import type { CustomTemplateDetail } from '../queries/customTemplates'

const saveAsTemplateSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  description: z.string().trim().optional(),
})
type SaveAsTemplateFormValues = z.infer<typeof saveAsTemplateSchema>

// Shown after the save succeeds: which env keys need a real value.
// No secret value is ever captured, only the key names.
function SavedTemplateSummary({ result }: { result: CustomTemplateDetail }) {
  const keys = result.required_env_keys ?? []
  return (
    <div className="space-y-3">
      <Alert>
        <CheckCircleIcon aria-hidden="true" />
        <AlertDescription>
          &ldquo;{result.name}&rdquo; is saved. It now appears under &ldquo;Your
          templates&rdquo; in the template marketplace, ready to deploy again.
        </AlertDescription>
      </Alert>
      <div className="space-y-1.5 rounded-md border border-border p-3 text-sm">
        <p className="font-medium text-foreground">
          {keys.length > 0
            ? `${keys.length} env var${keys.length === 1 ? '' : 's'} will need a value`
            : 'No secret values were captured'}
        </p>
        <p className="text-xs text-muted-foreground">
          Secret, database, and vault-backed env values are never copied into a
          template, only the key name. Deploying this template will ask for a
          real value before it can start, the same way a built-in catalog entry
          that needs configuration already does.
        </p>
        {keys.length > 0 ? (
          <div className="flex flex-wrap gap-1.5 pt-1">
            {keys.map((key) => (
              <Badge key={key} variant="outline" className="font-mono text-xs">
                {key}
              </Badge>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}

// One app's save-as-template action, next to CloneAppDialog. Unlike
// clone, this never creates another app or navigates away; the dialog
// shows the result in place until dismissed.
export function SaveAsTemplateDialog({
  appName,
  control,
}: {
  appName: string
  control?: DialogControl
}) {
  const [internalOpen, setInternalOpen] = useState(false)
  const open = control?.open ?? internalOpen
  const setOpen = (next: boolean) => {
    setInternalOpen(next)
    control?.onOpenChange?.(next)
  }
  const saveAsTemplate = useSaveAppAsTemplate()
  const { register, handleSubmit, formState, reset } =
    useForm<SaveAsTemplateFormValues>({
      resolver: zodResolver(saveAsTemplateSchema),
      defaultValues: { name: appName, description: '' },
    })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset({ name: appName, description: '' })
      saveAsTemplate.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    saveAsTemplate.mutate(
      {
        appName,
        request: {
          name: values.name.trim(),
          description: values.description?.trim() || undefined,
        },
      },
      {
        onSuccess: () => {
          toast.add({
            title: `Saved "${values.name.trim()}" as a template.`,
            type: 'success',
          })
        },
      },
    )
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {control?.hideTrigger ? null : (
        <DialogTrigger
          render={<Button type="button" variant="outline" size="sm" />}
        >
          <PackageIcon className="size-3.5" aria-hidden="true" />
          Save as template
        </DialogTrigger>
      )}
      <DialogContent className="sm:max-w-sm">
        {saveAsTemplate.data ? (
          <>
            <DialogHeader>
              <DialogTitle>Template saved</DialogTitle>
              <DialogDescription>
                &ldquo;{appName}&rdquo;&apos;s current configuration is now a
                reusable template.
              </DialogDescription>
            </DialogHeader>
            <SavedTemplateSummary result={saveAsTemplate.data} />
            <DialogFooter>
              <Button
                type="button"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                Done
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>
                Save &ldquo;{appName}&rdquo; as a template
              </DialogTitle>
              <DialogDescription>
                Captures this app&apos;s image, ports, env, volumes, and command
                as a one-click template you or a teammate can deploy again.
                Secret, database, and vault-backed env values are never
                captured, only their key names.
              </DialogDescription>
            </DialogHeader>
            <form
              onSubmit={(e) => {
                void onSubmit(e)
              }}
              className="space-y-4"
            >
              <Field>
                <FieldLabel htmlFor="save-as-template-name">
                  Template name
                </FieldLabel>
                <Input id="save-as-template-name" {...register('name')} />
                <FieldError errors={[formState.errors.name]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="save-as-template-description">
                  Description (optional)
                </FieldLabel>
                <Textarea
                  id="save-as-template-description"
                  rows={3}
                  placeholder="What this stack is, so a teammate recognizes it later"
                  {...register('description')}
                />
              </Field>

              {saveAsTemplate.isError ? (
                <p className="flex items-start gap-1.5 text-sm text-destructive">
                  <WarningIcon
                    className="mt-0.5 size-4 shrink-0"
                    aria-hidden="true"
                  />
                  {saveAsTemplate.error.message}
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
                  Cancel
                </Button>
                <Button type="submit" disabled={saveAsTemplate.isPending}>
                  {saveAsTemplate.isPending ? 'Saving...' : 'Save template'}
                </Button>
              </DialogFooter>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
