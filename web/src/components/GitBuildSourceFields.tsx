import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Controller,
  type Control,
  type FormState,
  type UseFormGetValues,
  type UseFormRegister,
  type UseFormSetValue,
  type UseFormWatch,
} from 'react-hook-form'
import { LoaderGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
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
import { BuildArgsFields } from './GitBuildArgsFields'

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
  pickedFromPicker = false,
  onDetected,
}: {
  control: Control<FormInput, unknown, FormOutput>
  register: UseFormRegister<FormInput>
  formState: FormState<FormInput>
  getValues: UseFormGetValues<FormInput>
  setValue: UseFormSetValue<FormInput>
  watch: UseFormWatch<FormInput>
  disabled: boolean
  /** True once the source picker supplied the repo, so the raw fields fold into a summary. */
  pickedFromPicker?: boolean
  /** Called each time framework pre-flight detection settles or resets
   *  to null; CreateAppFromGitFields stores the latest result to send
   *  with the build trigger. */
  onDetected: (result: DetectFrameworkResult | null) => void
}) {
  const { t } = useTranslation('gitBuild')
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
  const [editSource, setEditSource] = useState(false)
  const summarized =
    pickedFromPicker &&
    repoUrl !== '' &&
    !editSource &&
    !formState.errors.repoUrl &&
    !formState.errors.ref
  const detection = useAutoDetectFramework({
    enabled: buildType !== 'image',
    repoUrl,
    ref,
    onDetected,
  })

  return (
    <>
      {buildType !== 'image' && summarized ? (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2 text-sm">
          {hostIcon ? (
            <BrandIcon name={hostIcon} className="size-4 shrink-0" />
          ) : null}
          <span className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
            {repoUrl.replace(/^https?:\/\//, '').replace(/\.git$/, '')}
            <span className="text-muted-foreground">
              {' '}
              @ {ref || t('sourceFields.defaultRef')}
            </span>
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={disabled}
            onClick={() => {
              setEditSource(true)
            }}
          >
            {t('sourceFields.edit')}
          </Button>
        </div>
      ) : null}

      {buildType !== 'image' && !summarized ? (
        <>
          <Field>
            <FieldLabel htmlFor="git-app-repo-url">
              {t('sourceFields.repoUrl')}
            </FieldLabel>
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
                  <LoaderGlassIcon className="size-5 animate-spin" />
                ) : null}
                {t('sourceFields.loadBranches')}
              </Button>
            </div>
            <FieldError errors={[formState.errors.repoUrl]} />
          </Field>

          {branchesQuery.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertDescription>
                {t('sourceFields.branchesFailed', {
                  message: branchesQuery.error.message,
                })}
              </AlertDescription>
            </Alert>
          ) : null}

          {branches.length > 0 ? (
            <Field>
              <FieldLabel htmlFor="git-app-branch-picker">
                {t('sourceFields.branch')}
              </FieldLabel>
              <Combobox
                id="git-app-branch-picker"
                value=""
                options={branches.map((b) => ({ value: b, label: b }))}
                placeholder={t('sourceFields.pickBranch')}
                searchPlaceholder={t('sourceFields.searchBranches')}
                triggerClassName="font-mono"
                onValueChange={(branch) => {
                  setValue('ref', branch, {
                    shouldValidate: true,
                    shouldDirty: true,
                  })
                }}
              />
            </Field>
          ) : null}

          <Field>
            <FieldLabel htmlFor="git-app-ref">
              {branches.length > 0
                ? t('sourceFields.refWithPicker')
                : t('sourceFields.ref')}
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
          <FieldLabel htmlFor="git-app-build-type">
            {t('sourceFields.buildPack')}
          </FieldLabel>
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
                className="grid h-auto w-full grid-cols-2 sm:grid-cols-4"
              >
                <TabsTrigger value="railpack" disabled={disabled}>
                  {t('sourceFields.autoDetect')}
                </TabsTrigger>
                <TabsTrigger value="dockerfile" disabled={disabled}>
                  {t('sourceFields.dockerfile')}
                </TabsTrigger>
                <TabsTrigger value="static" disabled={disabled}>
                  {t('sourceFields.staticSite')}
                </TabsTrigger>
                <TabsTrigger value="image" disabled={disabled}>
                  {t('sourceFields.prebuiltImage')}
                </TabsTrigger>
              </TabsList>
              <TabsContent value="railpack" className="pt-2">
                <FieldDescription>
                  {t('sourceFields.autoDetectHelp')}
                </FieldDescription>
              </TabsContent>
              <TabsContent value="dockerfile" className="pt-2">
                <FieldDescription>
                  {t('sourceFields.dockerfileHelp')}
                </FieldDescription>
              </TabsContent>
              <TabsContent value="static" className="pt-2">
                <FieldDescription>
                  {t('sourceFields.staticHelp')}
                </FieldDescription>
              </TabsContent>
              <TabsContent value="image" className="pt-2">
                <FieldDescription>
                  {t('sourceFields.imageHelp')}
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
            <FieldLabel htmlFor="git-app-image">
              {t('sourceFields.imageRef')}
            </FieldLabel>
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
              {t('sourceFields.imageRefHelp')}
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
            {showAdvanced
              ? t('sourceFields.hideAdvanced')
              : t('sourceFields.showAdvanced')}
          </button>

          {showAdvanced ? (
            <div className="mt-3 flex flex-col gap-4 rounded-lg border border-dashed border-border p-3">
              <div className="flex flex-col gap-4 sm:flex-row">
                {buildType !== 'static' ? (
                  <Field className="flex-1">
                    <FieldLabel htmlFor="git-app-image-repo">
                      {t('sourceFields.imageName')}
                    </FieldLabel>
                    <Input
                      id="git-app-image-repo"
                      className="font-mono"
                      placeholder={t('sourceFields.imageNamePlaceholder')}
                      autoComplete="off"
                      spellCheck={false}
                      disabled={disabled}
                      {...register('imageRepo')}
                    />
                    <FieldDescription>
                      {t('sourceFields.imageNameHelp')}
                    </FieldDescription>
                  </Field>
                ) : null}

                {buildType === 'dockerfile' ? (
                  <Field className="flex-1">
                    <FieldLabel htmlFor="git-app-dockerfile-path">
                      {t('sourceFields.dockerfilePath')}
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
                      {t('sourceFields.dockerfilePathHelp')}
                    </FieldDescription>
                  </Field>
                ) : null}

                <Field className="flex-1">
                  <FieldLabel htmlFor="git-app-base-directory">
                    {t('sourceFields.baseDirectory')}
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
                    {t('sourceFields.baseDirectoryHelp')}
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
