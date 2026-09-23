import { useState } from 'react'
import {
  Controller,
  useFieldArray,
  type Control,
  type FormState,
  type UseFormGetValues,
  type UseFormRegister,
  type UseFormSetValue,
  type UseFormWatch,
} from 'react-hook-form'
import {
  WarningIcon,
  SpinnerIcon,
  PlusIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { useGitBranches } from '../queries/gitBranches'
import { useAutoDetectFramework } from '../hooks/useAutoDetectFramework'
import type { DetectFrameworkResult } from '../queries/buildDetect'
import { gitHostIconName } from '../lib/gitHost'
import { BrandIcon } from './BrandIcon'
import { DetectionStatus } from './DetectionStatus'
import type { FormInput, FormOutput } from './CreateAppFromGitFields'
import { RegistryImagePicker } from './RegistryImagePicker'

// GitBuildSourceFields is CreateAppFromGitFields' git-source input
// group: repository URL, a branch picker, the build pack choice, a
// framework pre-flight detection status, and (build-type-dependent)
// Dockerfile path / base directory / build args. Dockerfile path only
// applies to build.type dockerfile; railpack has no path concept at
// all. Base directory applies to dockerfile, railpack, and static
// alike, since it scopes the whole build context for a monorepo.
export function GitBuildSourceFields({
  control,
  register,
  formState,
  getValues,
  setValue,
  watch,
  disabled,
  onDetected,
}: {
  control: Control<FormInput, unknown, FormOutput>
  register: UseFormRegister<FormInput>
  formState: FormState<FormInput>
  getValues: UseFormGetValues<FormInput>
  setValue: UseFormSetValue<FormInput>
  watch: UseFormWatch<FormInput>
  disabled: boolean
  /** Called each time framework pre-flight detection settles or resets
   *  to null; CreateAppFromGitFields stores the latest result to send
   *  with the build trigger. */
  onDetected: (result: DetectFrameworkResult | null) => void
}) {
  const buildType = watch('buildType')
  // null until "Load branches" is clicked: see useGitBranches's own doc
  // comment for why this isn't fired on every keystroke. Re-clicking
  // after editing the URL re-triggers a fresh fetch, since the query key
  // is the URL itself.
  const [loadedRepoUrl, setLoadedRepoUrl] = useState<string | null>(null)
  // Image name / Dockerfile path are overrides most first-time users
  // never need to touch, so they start collapsed. See the toggle below.
  const [showAdvanced, setShowAdvanced] = useState(false)
  const branchesQuery = useGitBranches(loadedRepoUrl)
  const branches = branchesQuery.data ?? []
  const hostIcon = gitHostIconName(watch('repoUrl'))

  const repoUrl = watch('repoUrl').trim()
  const ref = watch('ref').trim()
  const detection = useAutoDetectFramework({
    enabled: buildType !== 'image',
    repoUrl,
    ref,
    onDetected,
  })

  return (
    <>
      {buildType !== 'image' ? (
        <>
          <Field>
            <FieldLabel htmlFor="git-app-repo-url">Repository URL</FieldLabel>
            <div className="flex gap-2">
              <div className="relative flex-1">
                {hostIcon ? (
                  <BrandIcon
                    name={hostIcon}
                    className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
                  />
                ) : null}
                <Input
                  id="git-app-repo-url"
                  className={cn('font-mono', hostIcon ? 'pl-8' : undefined)}
                  placeholder="https://github.com/you/app.git"
                  autoComplete="off"
                  spellCheck={false}
                  disabled={disabled}
                  {...register('repoUrl')}
                />
              </div>
              <Button
                type="button"
                variant="outline"
                disabled={disabled || branchesQuery.isFetching}
                onClick={() => {
                  setLoadedRepoUrl(getValues('repoUrl').trim())
                }}
              >
                {branchesQuery.isFetching ? (
                  <SpinnerIcon className="size-4 animate-spin" />
                ) : null}
                Load branches
              </Button>
            </div>
            <FieldError errors={[formState.errors.repoUrl]} />
          </Field>

          {branchesQuery.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>
                {branchesQuery.error.message}. You can still type a branch, tag,
                or commit manually below.
              </AlertDescription>
            </Alert>
          ) : null}

          {branches.length > 0 ? (
            <Field>
              <FieldLabel htmlFor="git-app-branch-picker">Branch</FieldLabel>
              <Select
                value=""
                onValueChange={(branch: string | null) => {
                  if (!branch) return
                  setValue('ref', branch, {
                    shouldValidate: true,
                    shouldDirty: true,
                  })
                }}
              >
                <SelectTrigger
                  id="git-app-branch-picker"
                  className="w-full font-mono"
                >
                  <SelectValue placeholder="Pick a branch..." />
                </SelectTrigger>
                <SelectContent>
                  {branches.map((b) => (
                    <SelectItem key={b} value={b} className="font-mono">
                      {b}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          ) : null}

          <Field>
            <FieldLabel htmlFor="git-app-ref">
              {branches.length > 0
                ? 'Branch, tag, or commit (or pick above)'
                : 'Branch, tag, or commit'}
            </FieldLabel>
            <Input
              id="git-app-ref"
              className="font-mono"
              placeholder="main"
              autoComplete="off"
              spellCheck={false}
              disabled={disabled}
              {...register('ref')}
            />
            <FieldError errors={[formState.errors.ref]} />
          </Field>
        </>
      ) : null}

      <Field>
        <div className="flex items-center justify-between gap-2">
          <FieldLabel htmlFor="git-app-build-type">Build pack</FieldLabel>
          {buildType !== 'image' ? (
            <DetectionStatus
              pending={detection.isPending}
              result={detection.result}
            />
          ) : null}
        </div>
        <Controller
          control={control}
          name="buildType"
          render={({ field }) => (
            <Tabs
              value={field.value}
              onValueChange={(v: unknown) => {
                if (
                  v === 'railpack' ||
                  v === 'dockerfile' ||
                  v === 'static' ||
                  v === 'image'
                ) {
                  field.onChange(v)
                }
              }}
            >
              {/* Only these four tabs: the four build.type cases
                  internal/deploy.Pipeline.Deploy actually has a case for
                  (internal/api/builds.go's handleTriggerBuild). No Nixpacks
                  (this project uses Railpack instead of Nixpacks)
                  and no Compose (internal/deploy's own compose case still
                  returns "not yet supported"). Same order and tab layout
                  GitSourceCard.tsx already uses for this exact choice,
                  which never offers "image" (see that component's own
                  build-type restrictions: a persistent git source has
                  nothing to redeploy for a pinned image). */}
              <TabsList
                id="git-app-build-type"
                className="grid w-full grid-cols-4"
              >
                <TabsTrigger value="railpack" disabled={disabled}>
                  Auto-detect
                </TabsTrigger>
                <TabsTrigger value="dockerfile" disabled={disabled}>
                  Dockerfile
                </TabsTrigger>
                <TabsTrigger value="static" disabled={disabled}>
                  Static site
                </TabsTrigger>
                <TabsTrigger value="image" disabled={disabled}>
                  Prebuilt image
                </TabsTrigger>
              </TabsList>
              <TabsContent value="railpack" className="pt-2">
                <FieldDescription>
                  Recommended. Detects Node, Go, and Java projects and builds
                  them automatically, no Dockerfile needed.
                </FieldDescription>
              </TabsContent>
              <TabsContent value="dockerfile" className="pt-2">
                <FieldDescription>
                  Builds from a Dockerfile in the repository.
                </FieldDescription>
              </TabsContent>
              <TabsContent value="static" className="pt-2">
                <FieldDescription>
                  Serves the checkout directly, no container.
                </FieldDescription>
              </TabsContent>
              <TabsContent value="image" className="pt-2">
                <FieldDescription>
                  Deploys an already-built image from a registry directly, no
                  git repository or build step at all.
                </FieldDescription>
              </TabsContent>
            </Tabs>
          )}
        />
      </Field>

      {buildType === 'image' ? (
        <>
          <RegistryImagePicker
            disabled={disabled}
            onSelect={(imageRef) => {
              setValue('image', imageRef, {
                shouldValidate: true,
                shouldDirty: true,
              })
            }}
          />
          <Field>
            <FieldLabel htmlFor="git-app-image">Image reference</FieldLabel>
            <Input
              id="git-app-image"
              className="font-mono"
              placeholder="registry.example.com/org/app:v1.2.3"
              autoComplete="off"
              spellCheck={false}
              disabled={disabled}
              {...register('image')}
            />
            <FieldDescription>
              A full registry reference, already built and pushed elsewhere.
              Deployed as-is.
            </FieldDescription>
            <FieldError errors={[formState.errors.image]} />
          </Field>
        </>
      ) : null}

      {buildType !== 'image' ? (
        <div>
          <button
            type="button"
            onClick={() => {
              setShowAdvanced((prev) => !prev)
            }}
            aria-expanded={showAdvanced}
            disabled={disabled}
            className="w-fit text-xs font-medium text-muted-foreground hover:text-foreground"
          >
            {showAdvanced ? 'Hide advanced options' : 'Show advanced options'}
          </button>

          {showAdvanced ? (
            <div className="mt-3 flex flex-col gap-4 rounded-lg border border-dashed border-border p-3">
              <div className="flex flex-col gap-4 sm:flex-row">
                {buildType !== 'static' ? (
                  <Field className="flex-1">
                    <FieldLabel htmlFor="git-app-image-repo">
                      Image name (optional)
                    </FieldLabel>
                    <Input
                      id="git-app-image-repo"
                      className="font-mono"
                      placeholder="defaults to the app name"
                      autoComplete="off"
                      spellCheck={false}
                      disabled={disabled}
                      {...register('imageRepo')}
                    />
                    <FieldDescription>
                      The name given to the built image. Leave blank to use the
                      app name.
                    </FieldDescription>
                  </Field>
                ) : null}

                {buildType === 'dockerfile' ? (
                  <Field className="flex-1">
                    <FieldLabel htmlFor="git-app-dockerfile-path">
                      Dockerfile path (optional)
                    </FieldLabel>
                    <Input
                      id="git-app-dockerfile-path"
                      className="font-mono"
                      placeholder="./Dockerfile"
                      autoComplete="off"
                      spellCheck={false}
                      disabled={disabled}
                      {...register('dockerfilePath')}
                    />
                    <FieldDescription>
                      Where the Dockerfile lives in your repo. Leave blank if
                      it&rsquo;s at the root.
                    </FieldDescription>
                  </Field>
                ) : null}

                <Field className="flex-1">
                  <FieldLabel htmlFor="git-app-base-directory">
                    Base directory (optional)
                  </FieldLabel>
                  <Input
                    id="git-app-base-directory"
                    className="font-mono"
                    placeholder="apps/web"
                    autoComplete="off"
                    spellCheck={false}
                    disabled={disabled}
                    {...register('baseDirectory')}
                  />
                  <FieldDescription>
                    Subdirectory to build from, for a monorepo. Leave blank to
                    build from the repo root.
                  </FieldDescription>
                </Field>
              </div>

              {buildType === 'dockerfile' ? (
                <BuildArgsFields
                  control={control}
                  register={register}
                  formState={formState}
                  disabled={disabled}
                />
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </>
  )
}

// BuildArgsFields is GitBuildSourceFields' Dockerfile-build-args editor,
// the same add/remove-row pattern LabelsEditor.tsx establishes for Docker
// labels, bound to this form's own control/register instead of its own.
function BuildArgsFields({
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
  const { fields, append, remove } = useFieldArray({
    control,
    name: 'buildArgs',
  })

  return (
    <Field>
      <FieldLabel>Build args (optional)</FieldLabel>
      <FieldDescription>
        Dockerfile <code className="font-mono">ARG</code> values passed to the
        build, for example a base image version or a build-time feature flag.
      </FieldDescription>
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
                aria-label="Build arg key"
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
              placeholder="value"
              autoComplete="off"
              spellCheck={false}
              disabled={disabled}
              aria-label="Build arg value"
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
              <XIcon />
              <span className="sr-only">Remove build arg</span>
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
        <PlusIcon />
        Add build arg
      </Button>
    </Field>
  )
}
