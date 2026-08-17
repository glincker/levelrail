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

// GitBuildSourceFields is CreateAppFromGitFields' git-source input group:
// repo URL, branch picker, build pack choice, and (dockerfile only) the
// Dockerfile path. The advanced Dockerfile-path field is hidden for
// railpack/image since neither has a build.path concept server-side.
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
              {/* No Nixpacks or Compose tabs: neither is a supported
                  build.type on the deploy pipeline yet. */}
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
