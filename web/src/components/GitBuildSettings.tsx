import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SpinnerIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from '@/components/ui/toast'
import { GitBuildSuggestions } from './GitBuildSuggestions'
import {
  useGitBuildDetection,
  useSetGitBuild,
} from '../queries/gitBuildSettings'
import {
  resolveBuildView,
  validateBuildPaths,
  type BuildPathProblem,
} from '../lib/gitBuildPaths'
import type {
  GitBuildSuggestion,
  GitSourceBuildType,
  GitSourceResource,
} from '../types/gitSource'

const BUILD_TYPES: GitSourceBuildType[] = ['railpack', 'dockerfile', 'static']

function isBuildType(value: unknown): value is GitSourceBuildType {
  return value === 'railpack' || value === 'dockerfile' || value === 'static'
}

interface GitBuildSettingsProps {
  appName: string
  source: GitSourceResource
}

// GitBuildSettings is the Source tab's build section: what a push builds,
// in plain words, one visible way to change it, and the repository's own
// Dockerfiles and build roots to pick from.
export function GitBuildSettings({ appName, source }: GitBuildSettingsProps) {
  const { t } = useTranslation('gitBuild')
  const save = useSetGitBuild(appName)
  const [editing, setEditing] = useState(
    () =>
      typeof window !== 'undefined' &&
      window.location.hash === '#build-settings',
  )
  const [buildType, setBuildType] = useState<GitSourceBuildType>(
    source.build_type,
  )
  const [dockerfile, setDockerfile] = useState(source.build_path ?? '')
  const [baseDirectory, setBaseDirectory] = useState(
    source.base_directory ?? '',
  )
  const [serverError, setServerError] = useState<string | null>(null)

  const watchRoot =
    source.build_type === 'railpack' && !(source.base_directory ?? '')
  const detection = useGitBuildDetection(
    appName,
    source.branch,
    editing || watchRoot,
  )

  const view = resolveBuildView(
    source.build_type,
    source.base_directory ?? '',
    source.build_path ?? '',
  )
  const preview = resolveBuildView(buildType, baseDirectory, dockerfile)
  const problem = validateBuildPaths(buildType, baseDirectory, dockerfile)
  const root = t('values.repoRoot')

  function openForm() {
    setBuildType(source.build_type)
    setDockerfile(source.build_path ?? '')
    setBaseDirectory(source.base_directory ?? '')
    setServerError(null)
    setEditing(true)
  }

  function persist(req: {
    build_type: GitSourceBuildType
    build_path: string
    base_directory: string
  }) {
    setServerError(null)
    save.mutate(req, {
      onSuccess: () => {
        setEditing(false)
        toast.add({ title: t('saved'), type: 'success' })
      },
      onError: (error) => {
        setServerError(error.message)
        toast.add({
          title: t('saveFailed'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  function applySuggestion(s: GitBuildSuggestion) {
    setBuildType(s.build_type)
    setDockerfile(s.dockerfile_path ?? '')
    setBaseDirectory(s.base_directory ?? '')
    persist({
      build_type: s.build_type,
      build_path: s.dockerfile_path ?? '',
      base_directory: s.base_directory ?? '',
    })
  }

  function problemText(p: BuildPathProblem): string {
    if (p === 'absolute') return t('form.invalidAbsolute')
    if (p === 'parent') return t('form.invalidParent')
    const example = baseDirectory.trim()
      ? `${baseDirectory.trim().replace(/\/+$/, '')}/Dockerfile`
      : 'apps/web/Dockerfile'
    return t('form.invalidOutside', { example })
  }

  function resolvedLine(
    type: GitSourceBuildType,
    v: ReturnType<typeof resolveBuildView>,
  ): string {
    const context = v.context ?? root
    if (type === 'dockerfile') {
      return t('resolved.dockerfile', {
        dockerfile: v.dockerfile ?? 'Dockerfile',
        where: context,
      })
    }
    return t(type === 'static' ? 'resolved.static' : 'resolved.railpack', {
      where: context,
    })
  }

  const showNudge =
    !editing && watchRoot && detection.data?.needs_build_settings === true

  return (
    <div id="build-settings" className="space-y-3 scroll-mt-20">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{t('title')}</h3>
        {!editing ? (
          <Button type="button" size="sm" onClick={openForm}>
            {t('change')}
          </Button>
        ) : null}
      </div>

      {showNudge ? (
        <div
          role="alert"
          className="flex flex-wrap items-start justify-between gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200"
        >
          <div className="flex min-w-0 flex-1 items-start gap-2">
            <WarningIcon className="mt-0.5 size-4 shrink-0" />
            <div>
              <p className="text-sm font-medium">{t('nudge.title')}</p>
              <p className="text-sm">{t('nudge.body')}</p>
            </div>
          </div>
          <Button type="button" size="sm" onClick={openForm}>
            {t('nudge.action')}
          </Button>
        </div>
      ) : null}

      <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-sm">
        <dt className="text-muted-foreground">{t('summary.buildPack')}</dt>
        <dd>{t(`packs.${source.build_type}`)}</dd>
        {source.build_type === 'static' ? (
          <>
            <dt className="text-muted-foreground">
              {t('summary.outputDirectory')}
            </dt>
            <dd className="font-mono">
              {source.build_path || t('values.notSet')}
            </dd>
          </>
        ) : (
          <>
            <dt className="text-muted-foreground">{t('summary.dockerfile')}</dt>
            <dd className="font-mono">
              {source.build_type === 'dockerfile'
                ? source.build_path ||
                  t('values.defaultDockerfile', {
                    path: view.dockerfile ?? 'Dockerfile',
                  })
                : t('values.notUsed')}
            </dd>
          </>
        )}
        <dt className="text-muted-foreground">{t('summary.baseDirectory')}</dt>
        <dd className="font-mono">{view.context ?? root}</dd>
        <dt className="text-muted-foreground">{t('summary.context')}</dt>
        <dd>{resolvedLine(source.build_type, view)}</dd>
      </dl>
      <p className="text-xs text-muted-foreground">{t('pathHelp')}</p>

      {editing ? (
        <div className="space-y-4 rounded-lg border border-input p-3">
          <Field>
            <FieldLabel htmlFor="git-build-type">
              {t('form.buildType')}
            </FieldLabel>
            <Tabs
              value={buildType}
              onValueChange={(v: unknown) => {
                if (isBuildType(v)) setBuildType(v)
              }}
            >
              <TabsList id="git-build-type" className="grid w-full grid-cols-3">
                {BUILD_TYPES.map((type) => (
                  <TabsTrigger
                    key={type}
                    value={type}
                    disabled={save.isPending}
                  >
                    {t(`packs.${type}`)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          </Field>

          {buildType !== 'railpack' ? (
            <Field>
              <FieldLabel htmlFor="git-build-dockerfile">
                {buildType === 'static'
                  ? t('form.outputDirectory')
                  : t('form.dockerfile')}
              </FieldLabel>
              <Input
                id="git-build-dockerfile"
                className="font-mono"
                placeholder="apps/web/Dockerfile"
                autoComplete="off"
                spellCheck={false}
                value={dockerfile}
                disabled={save.isPending}
                onChange={(e) => {
                  setDockerfile(e.target.value)
                }}
              />
              <FieldDescription>
                {buildType === 'static'
                  ? t('form.outputDirectoryHelp')
                  : t('form.dockerfileHelp')}
              </FieldDescription>
              {problem?.field === 'path' ? (
                <FieldError
                  errors={[{ message: problemText(problem.problem) }]}
                />
              ) : null}
            </Field>
          ) : null}

          <Field>
            <FieldLabel htmlFor="git-build-base-directory">
              {t('form.baseDirectory')}
            </FieldLabel>
            <Input
              id="git-build-base-directory"
              className="font-mono"
              placeholder="apps/web"
              autoComplete="off"
              spellCheck={false}
              value={baseDirectory}
              disabled={save.isPending}
              onChange={(e) => {
                setBaseDirectory(e.target.value)
              }}
            />
            <FieldDescription>{t('form.baseDirectoryHelp')}</FieldDescription>
            {problem?.field === 'base' ? (
              <FieldError
                errors={[{ message: problemText(problem.problem) }]}
              />
            ) : null}
          </Field>

          <p className="text-sm">
            <span className="text-muted-foreground">
              {t('form.willBuild')}:{' '}
            </span>
            {resolvedLine(buildType, preview)}
          </p>

          <GitBuildSuggestions
            branch={source.branch}
            detection={detection.data}
            loading={detection.isFetching}
            error={detection.error}
            applying={save.isPending}
            onRetry={() => {
              void detection.refetch()
            }}
            onApply={applySuggestion}
          />

          {serverError ? (
            <FieldError errors={[{ message: serverError }]} />
          ) : null}

          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              disabled={save.isPending || problem !== null}
              onClick={() => {
                persist({
                  build_type: buildType,
                  build_path: buildType === 'railpack' ? '' : dockerfile.trim(),
                  base_directory: baseDirectory.trim(),
                })
              }}
            >
              {save.isPending ? (
                <SpinnerIcon className="size-4 animate-spin" />
              ) : null}
              {save.isPending ? t('saving') : t('save')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={save.isPending}
              onClick={() => {
                setEditing(false)
              }}
            >
              {t('cancel')}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  )
}
