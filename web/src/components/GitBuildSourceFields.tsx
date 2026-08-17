import { useState } from 'react'
import {
  Controller,
  type Control,
  type FormState,
  type UseFormGetValues,
  type UseFormRegister,
  type UseFormSetValue,
  type UseFormWatch,
} from 'react-hook-form'
import { WarningIcon, SpinnerIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useGitBranches } from '../queries/gitBranches'
import type { FormInput, FormOutput } from './CreateAppFromGitFields'

// GitBuildSourceFields is CreateAppFromGitFields' git-source input
// group: repository URL, a real branch picker backed by
// GET-equivalent POST /api/v1/git/branches, the build pack choice, and
// (dockerfile only) the Dockerfile path. Split out once the parent
// form crossed a comfortable single-file size with this addition,
// mirroring HealthCheckEditor.tsx's own ProbeFields split: a
// sub-component that takes register/control/formState as explicit
// props typed against the owning form's exact value shape, not a
// FormProvider/context split (no component in this codebase uses one).
//
// The Dockerfile-path field only appears for build.type: dockerfile.
// build.type: static also has a meaningful build.path server-side
// (internal/deploy/static.go's deployStatic: the built output
// subdirectory to serve, relative to the checkout root), but this form
// deliberately doesn't expose it yet: a static site with output at the
// checkout root (no build step) is the common case this wizard targets
// first, and adding a second, differently-labeled path field for one
// build pack is a real scope expansion this pass didn't take. A static
// site whose output lives in a subdirectory still deploys fine through
// app.yaml, just not through this manual-trigger wizard yet.
// build.type: railpack and build.type: image both have no build.path
// concept at all (handleTriggerBuild rejects one being set for either),
// so neither ever shows the field.
//
// build.type: image needs no build at all (internal/deploy.Pipeline.
// deployImage saves the given reference directly), so its own tab shows
// an image-reference input instead of anything build-related. The
// repository URL/branch fields above still stay visible and required:
// handleTriggerBuild (internal/api/builds.go) clones the repo for every
// build.type today, this one included, even though deployImage never
// reads the checkout it produces.
export function GitBuildSourceFields({
  control,
  register,
  formState,
  getValues,
  setValue,
  watch,
  disabled,
}: {
  control: Control<FormInput, unknown, FormOutput>
  register: UseFormRegister<FormInput>
  formState: FormState<FormInput>
  getValues: UseFormGetValues<FormInput>
  setValue: UseFormSetValue<FormInput>
  watch: UseFormWatch<FormInput>
  disabled: boolean
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

  return (
    <>
      <Field>
        <FieldLabel htmlFor="git-app-repo-url">Repository URL</FieldLabel>
        <div className="flex gap-2">
          <Input
            id="git-app-repo-url"
            className="flex-1 font-mono"
            placeholder="https://github.com/you/app.git"
            autoComplete="off"
            spellCheck={false}
            disabled={disabled}
            {...register('repoUrl')}
          />
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
            <SelectTrigger id="git-app-branch-picker" className="w-full font-mono">
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

      <Field>
        <FieldLabel htmlFor="git-app-build-type">Build pack</FieldLabel>
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
                  (this project uses Railpack instead, see CLAUDE.md 4.4)
                  and no Compose (internal/deploy's own compose case still
                  returns "not yet supported"). Same order and tab layout
                  GitSourceCard.tsx already uses for the three build-from-
                  source cases; "Prebuilt image" has no GitSourceCard
                  counterpart, see normalizeGitSourceBuildType's own doc
                  comment (internal/api/git_sources.go) for why. */}
              <TabsList id="git-app-build-type" className="grid w-full grid-cols-4">
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
              <TabsContent value="image" className="space-y-3 pt-2">
                <FieldDescription>
                  Already built and pushed somewhere Docker can pull it from?
                  Skip the build entirely and deploy that image directly.
                </FieldDescription>
                <Field>
                  <FieldLabel htmlFor="git-app-image">Image</FieldLabel>
                  <Input
                    id="git-app-image"
                    className="font-mono"
                    placeholder="ghcr.io/you/app:latest"
                    autoComplete="off"
                    spellCheck={false}
                    disabled={disabled}
                    {...register('image')}
                  />
                  <FieldError errors={[formState.errors.image]} />
                </Field>
              </TabsContent>
            </Tabs>
          )}
        />
      </Field>

      {buildType !== 'static' && buildType !== 'image' ? (
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
            <div className="mt-3 flex flex-col gap-4 rounded-lg border border-dashed border-border p-3 sm:flex-row">
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
            </div>
          ) : null}
        </div>
      ) : null}
    </>
  )
}
