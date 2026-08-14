import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { RocketIcon } from '@phosphor-icons/react/dist/ssr'
import { useTriggerDeploy } from '../queries/deploys'
import { useImageTagsOptional } from '../queries/images'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

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
//
// Rendered above the tab shell in the app detail route (not inside any
// one tab) because triggering a deploy is the single most common action
// on this page and applies to the app as a whole, not to one config
// concern, matching how Coolify/Dokploy keep their "Deploy" control
// pinned in the resource header rather than buried in a settings tab.
export function DeployTriggerForm({ appName }: { appName: string }) {
  const triggerDeploy = useTriggerDeploy(appName)
  // Optional convenience only, the same graceful-degradation shape
  // useNodeListOptional's own doc comment establishes (queries/nodes.ts):
  // a failure or empty result here must never block this form's core
  // function, so the dropdown below is simply not rendered rather than
  // surfacing a loading or error state of its own. Manually typing an
  // image tag (e.g. one pushed outside this system) always keeps
  // working regardless.
  const imageTags = useImageTagsOptional(appName)
  const tags = imageTags.data ?? []
  const { register, handleSubmit, formState, reset, setValue } =
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
    <Card className="ring-primary/20">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RocketIcon className="size-4" />
          Trigger a deploy
        </CardTitle>
        <CardDescription>
          Points this app at an already-built image tag. This does not build
          anything and does not stream deploy progress.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {tags.length > 0 ? (
          <Field className="mb-3">
            <FieldLabel htmlFor="deploy-image-suggestion">
              Previously built
            </FieldLabel>
            <Select
              value=""
              onValueChange={(tag: string | null) => {
                if (!tag) return
                setValue('image', tag, {
                  shouldValidate: true,
                  shouldDirty: true,
                })
              }}
            >
              <SelectTrigger
                id="deploy-image-suggestion"
                className="w-full font-mono"
              >
                <SelectValue placeholder="Pick a previously-built tag..." />
              </SelectTrigger>
              <SelectContent>
                {tags.map((t) => (
                  <SelectItem key={t.tag} value={t.tag} className="font-mono">
                    {t.tag}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : null}
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="flex flex-col items-start gap-3 sm:flex-row"
        >
          <Field className="flex-1">
            <Input
              {...register('image')}
              className="font-mono"
              placeholder="registry.example.com/app:abc1234"
              autoComplete="off"
              spellCheck={false}
            />
            <FieldError
              errors={
                formState.errors.image ? [formState.errors.image] : undefined
              }
            />
          </Field>
          <Button type="submit" disabled={triggerDeploy.isPending}>
            {triggerDeploy.isPending ? 'Triggering...' : 'Deploy'}
          </Button>
        </form>
        {triggerDeploy.isError ? (
          <Alert variant="destructive" className="mt-3">
            <AlertDescription>{triggerDeploy.error.message}</AlertDescription>
          </Alert>
        ) : null}
        {triggerDeploy.isSuccess ? (
          <p className="mt-3 text-xs text-green-700 dark:text-green-400">
            Deploy triggered. Check the Overview tab for the outcome.
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
