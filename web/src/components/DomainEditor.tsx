import { zodResolver } from '@hookform/resolvers/zod'
import { useFieldArray, useForm } from 'react-hook-form'
import { z } from 'zod'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'

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
    updateApp.mutate({
      ...app,
      domains: values.domains.map((d) => d.value.trim()),
    })
  })

  return (
    <section className="rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
      <h2 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
        Domains
      </h2>
      <form
        onSubmit={(e) => {
          void onSubmit(e)
        }}
        className="mt-3 space-y-2"
      >
        {fields.length === 0 ? (
          <p className="text-sm text-neutral-500 dark:text-neutral-400">
            No domains configured.
          </p>
        ) : null}
        {fields.map((field, index) => (
          <div key={field.id} className="flex items-start gap-2">
            <div className="flex-1">
              <input
                {...register(`domains.${index}.value`)}
                className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1 text-sm dark:border-neutral-700 dark:bg-neutral-900"
                placeholder="app.example.com"
              />
              {formState.errors.domains?.[index]?.value ? (
                <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                  {formState.errors.domains[index]?.value?.message}
                </p>
              ) : null}
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
              append({ value: '' })
            }}
            className="rounded-md border border-neutral-300 px-2 py-1 text-xs text-neutral-700 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-900"
          >
            Add domain
          </button>
          <button
            type="submit"
            disabled={updateApp.isPending}
            className="rounded-md bg-neutral-900 px-3 py-1 text-xs font-medium text-white hover:bg-neutral-700 disabled:opacity-50 dark:bg-neutral-100 dark:text-neutral-900 dark:hover:bg-neutral-300"
          >
            {updateApp.isPending ? 'Saving...' : 'Save domains'}
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
