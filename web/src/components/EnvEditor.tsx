import { zodResolver } from '@hookform/resolvers/zod'
import { useFieldArray, useForm } from 'react-hook-form'
import { z } from 'zod'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'

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
    <section className="rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
      <h2 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
        Environment variables
      </h2>
      <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
        Plain string values only. Secret references are not supported yet.
      </p>
      <form
        onSubmit={(e) => {
          void onSubmit(e)
        }}
        className="mt-3 space-y-2"
      >
        {fields.length === 0 ? (
          <p className="text-sm text-neutral-500 dark:text-neutral-400">
            No environment variables set.
          </p>
        ) : null}
        {fields.map((field, index) => (
          <div key={field.id} className="flex items-start gap-2">
            <div className="flex-1">
              <input
                {...register(`vars.${index}.key`)}
                className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1 font-mono text-sm dark:border-neutral-700 dark:bg-neutral-900"
                placeholder="KEY"
              />
              {formState.errors.vars?.[index]?.key ? (
                <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                  {formState.errors.vars[index]?.key?.message}
                </p>
              ) : null}
            </div>
            <div className="flex-1">
              <input
                {...register(`vars.${index}.value`)}
                className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1 font-mono text-sm dark:border-neutral-700 dark:bg-neutral-900"
                placeholder="value"
              />
            </div>
            <button
              type="button"
              onClick={() => {
                remove(index)
              }}
              className="rounded-md border border-neutral-300 px-2 py-1 text-xs text-neutral-600 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-400 dark:hover:bg-neutral-900"
            >
              Remove
            </button>
          </div>
        ))}
        <div className="flex items-center gap-2 pt-1">
          <button
            type="button"
            onClick={() => {
              append({ key: '', value: '' })
            }}
            className="rounded-md border border-neutral-300 px-2 py-1 text-xs text-neutral-700 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-900"
          >
            Add variable
          </button>
          <button
            type="submit"
            disabled={updateApp.isPending}
            className="rounded-md bg-neutral-900 px-3 py-1 text-xs font-medium text-white hover:bg-neutral-700 disabled:opacity-50 dark:bg-neutral-100 dark:text-neutral-900 dark:hover:bg-neutral-300"
          >
            {updateApp.isPending ? 'Saving...' : 'Save variables'}
          </button>
        </div>
        {updateApp.isError ? (
          <p className="text-xs text-red-600 dark:text-red-400">
            {updateApp.error.message}
          </p>
        ) : null}
        {updateApp.isSuccess ? (
          <p className="text-xs text-green-700 dark:text-green-400">Saved.</p>
        ) : null}
      </form>
    </section>
  )
}
