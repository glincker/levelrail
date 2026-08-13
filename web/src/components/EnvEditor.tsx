import { zodResolver } from '@hookform/resolvers/zod'
import { useFieldArray, useForm } from 'react-hook-form'
import { z } from 'zod'
import { PlusIcon, VariableIcon, XIcon } from 'lucide-react'
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

const envSchema = z
  .object({
    vars: z.array(
      z.object({
        key: z.string().trim().min(1, 'Key is required'),
        value: z.string(),
      }),
    ),
  })
  .superRefine((data, ctx) => {
    const seen = new Set<string>()
    data.vars.forEach((v, index) => {
      const key = v.key.trim()
      if (key && seen.has(key)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Duplicate key',
          path: ['vars', index, 'key'],
        })
      }
      seen.add(key)
    })
  })

type EnvFormValues = z.infer<typeof envSchema>

function toFieldValues(
  env?: Record<string, string>,
): { key: string; value: string }[] {
  return Object.entries(env ?? {}).map(([key, value]) => ({ key, value }))
}

// Same add/remove-list, full-replace-PUT pattern as DomainEditor, bound
// to AppDetail.env instead of .domains. Deliberately plain string
// values only: internal/deploy.requireNoUnresolvedEnv (TASKS.md 1.7)
// still rejects app.yaml's `{ secret: true }` / `{ from: ... }` env
// references outright, and store.DesiredService.Env is a flat
// map[string]string with no room to represent a secret reference even
// if this editor tried. The help text below says so explicitly so this
// doesn't read as secret support that just isn't wired up yet.
export function EnvEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const { control, register, handleSubmit, formState } = useForm<EnvFormValues>(
    {
      resolver: zodResolver(envSchema),
      values: { vars: toFieldValues(app.env) },
      resetOptions: { keepDirtyValues: true },
    },
  )
  const { fields, append, remove } = useFieldArray({ control, name: 'vars' })

  const onSubmit = handleSubmit((values) => {
    const env: Record<string, string> = {}
    for (const v of values.vars) {
      env[v.key.trim()] = v.value
    }
    updateApp.mutate({ ...app, env })
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <VariableIcon className="size-4" />
          Environment variables
        </CardTitle>
        <CardDescription>
          Plain string values only. Secret references are not supported yet, use
          the Secrets card below for values that need to stay encrypted.
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
              No environment variables set.
            </p>
          ) : (
            <FieldGroup className="gap-2">
              {fields.map((field, index) => (
                <Field key={field.id} orientation="responsive">
                  <div className="flex-1">
                    <Input
                      {...register(`vars.${index}.key`)}
                      className="font-mono"
                      placeholder="KEY"
                    />
                    <FieldError
                      errors={
                        formState.errors.vars?.[index]?.key
                          ? [formState.errors.vars[index]?.key]
                          : undefined
                      }
                    />
                  </div>
                  <div className="flex flex-1 items-start gap-2">
                    <Input
                      {...register(`vars.${index}.value`)}
                      className="flex-1 font-mono"
                      placeholder="value"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => {
                        remove(index)
                      }}
                    >
                      <XIcon />
                      <span className="sr-only">Remove variable</span>
                    </Button>
                  </div>
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
                append({ key: '', value: '' })
              }}
            >
              <PlusIcon />
              Add variable
            </Button>
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save variables'}
            </Button>
          </div>
          {updateApp.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{updateApp.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {updateApp.isSuccess ? (
            <p className="text-xs text-green-700 dark:text-green-400">Saved.</p>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
