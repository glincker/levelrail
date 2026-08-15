import { zodResolver } from '@hookform/resolvers/zod'
import { useFieldArray, useForm } from 'react-hook-form'
import { z } from 'zod'
import { GlobeIcon, PlusIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldError, FieldGroup } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'

const domainSchema = z.object({
  domains: z.array(
    z.object({
      value: z
        .string()
        .trim()
        .min(1, 'Domain is required')
        .regex(
          /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/i,
          'Enter a valid domain, e.g. app.example.com',
        ),
    }),
  ),
})

type DomainFormValues = z.infer<typeof domainSchema>

function toFieldValues(domains?: string[]): { value: string }[] {
  return (domains ?? []).map((value) => ({ value }))
}

// A simple add/remove list bound to AppDetail.domains, saved via PUT
// /api/v1/apps/{name} (internal/api/apps.go's handleUpdateApp). That
// handler is a full replace, not a patch, so every submit here spreads
// the current `app` and swaps only `domains`, sending the complete
// resource back rather than a domains-only body.
//
// `values` + `resetOptions.keepDirtyValues` (rather than a manual
// useEffect/reset) is what keeps this safe to render next to
// EnvEditor: both editors share the same AppDetail prop, and a
// successful save in one must not silently discard unsaved edits
// sitting in the other. keepDirtyValues re-syncs untouched fields to
// fresh server data while leaving anything the operator has actually
// edited alone.
export function DomainEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const { control, register, handleSubmit, formState } =
    useForm<DomainFormValues>({
      resolver: zodResolver(domainSchema),
      values: { domains: toFieldValues(app.domains) },
      resetOptions: { keepDirtyValues: true },
    })
  const { fields, append, remove } = useFieldArray({
    control,
    name: 'domains',
  })

  const onSubmit = handleSubmit((values) => {
    updateApp.mutate(
      {
        ...app,
        domains: values.domains.map((d) => d.value.trim()),
      },
      {
        onSuccess: () => {
          toast.add({ title: 'Domains saved.', type: 'success' })
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GlobeIcon className="size-4" />
          Domains
        </CardTitle>
        <CardDescription>
          Domains routed to this app through the ingress layer.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-3"
        >
          {fields.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No domains configured.
            </p>
          ) : (
            <FieldGroup className="gap-2">
              {fields.map((field, index) => (
                <Field key={field.id} orientation="horizontal">
                  <div className="flex-1">
                    <Input
                      {...register(`domains.${index}.value`)}
                      className="font-mono"
                      placeholder="app.example.com"
                      aria-label="Domain"
                    />
                    <FieldError
                      errors={
                        formState.errors.domains?.[index]?.value
                          ? [formState.errors.domains[index]?.value]
                          : undefined
                      }
                    />
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => {
                      remove(index)
                    }}
                  >
                    <XIcon />
                    <span className="sr-only">Remove domain</span>
                  </Button>
                </Field>
              ))}
            </FieldGroup>
          )}
          <div className="flex items-center gap-2 pt-1">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                append({ value: '' })
              }}
            >
              <PlusIcon />
              Add domain
            </Button>
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save domains'}
            </Button>
          </div>
          {updateApp.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{updateApp.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
