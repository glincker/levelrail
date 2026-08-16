import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useNavigate } from '@tanstack/react-router'
import { GitBranchIcon, RocketIcon } from '@phosphor-icons/react/dist/ssr'
import { useTriggerDeploy } from '../queries/deploys'
import { useTriggerBuild } from '../queries/builds'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from '@/components/ui/toast'

const triggerSchema = z.object({
  image: z.string().trim().min(1, 'Image tag is required'),
})

type TriggerFormValues = z.infer<typeof triggerSchema>

// DeployExistingImageForm is the original, unchanged "point this app at
// an already-built image tag" path (POST /api/v1/apps/{name}/deploys,
// internal/api/deploys.go's handleTriggerDeploy). That handler's own doc
// comment is explicit about what this does and does not do: it only
// points desired_services.image at a new tag for the application
// controller's next reconcile to pick up. Nothing here builds an image,
// and there is no backend SSE endpoint yet to stream deploy progress, so
// this form deliberately has no log viewer, no progress bar, and no
// "deploying..." state beyond the mutation's own pending flag: any of
// those would imply a live process this endpoint doesn't actually drive.
function DeployExistingImageForm({ appName }: { appName: string }) {
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
        toast.add({
          title: 'Deploy triggered.',
          description: 'Check the Overview tab for the outcome.',
          type: 'success',
        })
      },
    })
  })

  return (
    <div>
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
    </div>
  )
}

const buildSchema = z.object({
  repoUrl: z.string().trim().min(1, 'Repository URL is required'),
  ref: z.string().trim().min(1, 'Branch, tag, or commit is required'),
  imageRepo: z.string().trim(),
  dockerfilePath: z.string().trim(),
})

type BuildFormValues = z.infer<typeof buildSchema>

// BuildFromSourceForm is the new path this task adds:
// POST /api/v1/apps/{name}/builds (internal/api/builds.go's
// handleTriggerBuild), for an operator with no working git webhook
// configured (internal/webhook.Config's own doc comment: deliberately
// static, single-app configuration set at server startup, not something
// the dashboard can point at an arbitrary app). This invokes the same
// internal/deploy.Pipeline the webhook receiver uses, given a git source
// supplied right here instead of a GitHub push payload: no app in this
// codebase has a stored git/build config yet (confirmed via
// internal/store, internal/spec), so repo_url/ref/build settings are
// asked for on every submission rather than being a one-time setting a
// later call could omit.
//
// The build itself runs asynchronously on the server: this mutation
// resolves as soon as the deploy attempt is recorded, not once the build
// finishes (see queries/builds.ts's own doc comment), so its pending
// state is only ever brief. A successful trigger navigates to that
// attempt's live log view (the same route DeployAttemptsList's own
// "Logs" link uses), rather than leaving the operator to go find it.
function BuildFromSourceForm({ appName }: { appName: string }) {
  const navigate = useNavigate()
  const triggerBuild = useTriggerBuild(appName)
  const { register, handleSubmit, formState, reset } = useForm<BuildFormValues>(
    {
      resolver: zodResolver(buildSchema),
      defaultValues: {
        repoUrl: '',
        ref: '',
        imageRepo: '',
        dockerfilePath: '',
      },
    },
  )

  const onSubmit = handleSubmit((values) => {
    triggerBuild.mutate(
      {
        repoUrl: values.repoUrl.trim(),
        ref: values.ref.trim(),
        imageRepo: values.imageRepo.trim() || undefined,
        buildPath: values.dockerfilePath.trim() || undefined,
      },
      {
        onSuccess: (result) => {
          reset({ repoUrl: '', ref: '', imageRepo: '', dockerfilePath: '' })
          toast.add({
            title: 'Build triggered.',
            description: 'Watching the build log now.',
            type: 'success',
          })
          // See CreateAppFromGitFields.tsx's identical navigation for
          // why result.id can be empty (attempt recording itself
          // failed) and what that means here.
          if (result.id) {
            void navigate({
              to: '/apps/$name/deploys/$deployId/logs',
              params: { name: appName, deployId: result.id },
            })
          }
        },
      },
    )
  })

  return (
    <div>
      <form
        onSubmit={(e) => {
          void onSubmit(e)
        }}
        className="space-y-3"
      >
        <div className="flex flex-col items-start gap-3 sm:flex-row">
          <Field className="flex-[2]">
            <FieldLabel htmlFor="build-repo-url">Repository URL</FieldLabel>
            <Input
              id="build-repo-url"
              {...register('repoUrl')}
              className="font-mono"
              placeholder="https://github.com/you/app.git"
              autoComplete="off"
              spellCheck={false}
              disabled={triggerBuild.isPending}
            />
            <FieldError
              errors={
                formState.errors.repoUrl
                  ? [formState.errors.repoUrl]
                  : undefined
              }
            />
          </Field>
          <Field className="flex-1">
            <FieldLabel htmlFor="build-ref">Branch, tag, or commit</FieldLabel>
            <Input
              id="build-ref"
              {...register('ref')}
              className="font-mono"
              placeholder="main"
              autoComplete="off"
              spellCheck={false}
              disabled={triggerBuild.isPending}
            />
            <FieldError
              errors={formState.errors.ref ? [formState.errors.ref] : undefined}
            />
          </Field>
        </div>
        <div className="flex flex-col items-start gap-3 sm:flex-row">
          <Field className="flex-1">
            <FieldLabel htmlFor="build-image-repo">
              Image name (optional)
            </FieldLabel>
            <Input
              id="build-image-repo"
              {...register('imageRepo')}
              className="font-mono"
              placeholder={appName}
              autoComplete="off"
              spellCheck={false}
              disabled={triggerBuild.isPending}
            />
          </Field>
          <Field className="flex-1">
            <FieldLabel htmlFor="build-dockerfile-path">
              Dockerfile path (optional)
            </FieldLabel>
            <Input
              id="build-dockerfile-path"
              {...register('dockerfilePath')}
              className="font-mono"
              placeholder="./Dockerfile"
              autoComplete="off"
              spellCheck={false}
              disabled={triggerBuild.isPending}
            />
          </Field>
          <Button
            type="submit"
            disabled={triggerBuild.isPending}
            className="sm:self-end"
          >
            {triggerBuild.isPending ? 'Building...' : 'Build and deploy'}
          </Button>
        </div>
      </form>
      {triggerBuild.isPending ? (
        <Alert className="mt-3">
          <AlertDescription>Triggering the build...</AlertDescription>
        </Alert>
      ) : null}
      {triggerBuild.isError ? (
        <Alert variant="destructive" className="mt-3">
          <AlertDescription>{triggerBuild.error.message}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}

// Rendered above the tab shell in the app detail route (not inside any
// one tab) because triggering a deploy is the single most common action
// on this page and applies to the app as a whole, not to one config
// concern, matching how Coolify/Dokploy keep their "Deploy" control
// pinned in the resource header rather than buried in a settings tab.
//
// Two tabs, not two cards: both paths ultimately do the same thing (get
// a new image running), so this stays one "Trigger a deploy" surface
// with a choice of how to get the image, rather than doubling the
// vertical space this page's most-used control occupies. "Existing
// image" (the original path) stays the default tab: it is the faster,
// already-working path for anyone with a registry set up, and this
// change must not make it harder to reach.
export function DeployTriggerForm({ appName }: { appName: string }) {
  return (
    <Card className="ring-primary/20">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RocketIcon className="size-4" />
          Trigger a deploy
        </CardTitle>
        <CardDescription>
          Deploy an already-built image tag, or build one from a git source
          first.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="existing-image">
          <TabsList>
            <TabsTrigger value="existing-image">
              <RocketIcon className="size-3.5" data-icon="inline-start" />
              Existing image
            </TabsTrigger>
            <TabsTrigger value="build-from-source">
              <GitBranchIcon className="size-3.5" data-icon="inline-start" />
              Build from source
            </TabsTrigger>
          </TabsList>
          <TabsContent value="existing-image" className="mt-3">
            <DeployExistingImageForm appName={appName} />
          </TabsContent>
          <TabsContent value="build-from-source" className="mt-3">
            <BuildFromSourceForm appName={appName} />
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  )
}
