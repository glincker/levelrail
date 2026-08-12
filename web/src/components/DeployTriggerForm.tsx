import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useTriggerDeploy } from '../queries/deploys'

const triggerSchema = z.object({
  image: z.string().trim().min(1, 'Image tag is required'),
})

type TriggerFormValues = z.infer<typeof triggerSchema>

// POST /api/v1/apps/{name}/deploys (internal/api/deploys.go's
// handleTriggerDeploy). That handler's own doc comment is explicit
// about what this does and does not do: it only points
// desired_services.image at a new tag for the application controller's
// next reconcile to pick up. Nothing builds an image from this form,
// and there is no backend SSE endpoint yet to stream build or deploy
// progress, so this form deliberately has no log viewer, no progress
// bar, and no "deploying..." state beyond the mutation's own pending
// flag: any of those would imply a live process this endpoint doesn't
// actually drive.
export function DeployTriggerForm({ appName }: { appName: string }) {
  const triggerDeploy = useTriggerDeploy(appName)
  const { register, handleSubmit, formState, reset } =
    useForm<TriggerFormValues>({
      resolver: zodResolver(triggerSchema),
      defaultValues: { image: '' },
    })

  const onSubmit = handleSubmit((values) => {
    triggerDeploy.mutate(values.image.trim(), {
      onSuccess: () => {
        reset({ image: '' })
      },
    })
  })

  return (
    <section className="rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
      <h2 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
        Trigger a deploy
      </h2>
      <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
        Points this app at an already-built image tag. This does not build
        anything and does not stream deploy progress.
      </p>
      <form
        onSubmit={(e) => {
          void onSubmit(e)
        }}
        className="mt-3 flex items-start gap-2"
      >
        <div className="flex-1">
          <input
            {...register('image')}
            className="w-full rounded-md border border-neutral-300 bg-white px-2 py-1 font-mono text-sm dark:border-neutral-700 dark:bg-neutral-900"
            placeholder="registry.example.com/app:abc1234"
          />
          {formState.errors.image ? (
            <p className="mt-1 text-xs text-red-600 dark:text-red-400">
              {formState.errors.image.message}
            </p>
          ) : null}
        </div>
        <button
          type="submit"
          disabled={triggerDeploy.isPending}
          className="rounded-md bg-neutral-900 px-3 py-1 text-xs font-medium whitespace-nowrap text-white hover:bg-neutral-700 disabled:opacity-50 dark:bg-neutral-100 dark:text-neutral-900 dark:hover:bg-neutral-300"
        >
          {triggerDeploy.isPending ? 'Triggering...' : 'Deploy'}
        </button>
      </form>
      {triggerDeploy.isError ? (
        <p className="mt-2 text-xs text-red-600 dark:text-red-400">
          {triggerDeploy.error.message}
        </p>
      ) : null}
      {triggerDeploy.isSuccess ? (
        <p className="mt-2 text-xs text-green-700 dark:text-green-400">
          Deploy triggered. Check reconcile status above for the outcome.
        </p>
      ) : null}
    </section>
  )
}
