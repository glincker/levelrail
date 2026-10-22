import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useDebouncedValue } from '../../../hooks/useDebouncedValue'
import { useDetectFramework } from '../../../queries/buildDetect'
import { useGitBranches } from '../../../queries/gitBranches'
import { useBrand } from '@/hooks/useBrand'
import { useFunctionRuntimes } from '../../../queries/functions'
import { useFunctionStarters } from '../../../queries/functionKv'
import type { GitRepoItem } from '../../../types/gitRepos'
import { CopyButton } from '../FunctionTemplates'
import { FieldError, FieldHelp } from './FieldMessage'
import { isHttpUrl, type FieldErrors, type FlowValues } from './logic'
import type { Preset } from './presets'
import { RepoPicker } from './RepoPicker'

const URL_DEBOUNCE_MS = 500
const FALLBACK_BRANCH = 'main'

function UtilityCli({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('functions')
  const brand = useBrand()
  return (
    <p>
      {t('deploy.source.utilityCli')}:{' '}
      <code>
        {brand.BinaryName}-cli functions init {id}
      </code>
    </p>
  )
}

function StarterCode({ preset }: Readonly<{ preset: Preset }>) {
  const { t } = useTranslation('functions')
  const { data } = useFunctionRuntimes()
  const starters = useFunctionStarters()
  const utility =
    preset.kind === 'idea'
      ? starters.data?.find((s) => s.preset === preset.id)
      : undefined
  const files =
    utility?.files ??
    data?.runtimes.find((r) => r.id === preset.runtime)?.files ??
    []
  if (files.length === 0) return null
  return (
    <div className="min-w-0 space-y-2 rounded-lg border border-input bg-muted/40 p-3">
      <p className="text-sm font-medium">{t('deploy.source.starterTitle')}</p>
      <p className="text-xs text-muted-foreground">
        {utility
          ? t('deploy.source.utilityBody')
          : t('deploy.source.starterBody')}
      </p>
      {utility ? (
        <div className="space-y-1 text-xs">
          <p className="font-medium">{t('deploy.source.utilityEnv')}</p>
          <ul className="space-y-0.5 text-muted-foreground">
            {utility.env.map((e) => (
              <li key={e.name}>
                <code>{e.name}</code>
                {e.required
                  ? ` (${t('deploy.source.utilityRequired')})`
                  : ''}: {e.description}
              </li>
            ))}
          </ul>
          {utility.uses_kv ? <p>{t('deploy.source.utilityKv')}</p> : null}
          <UtilityCli id={utility.id} />
        </div>
      ) : null}
      {files.map((f) => (
        <div
          key={f.path}
          className="min-w-0 rounded-md border border-input bg-background"
        >
          <div className="flex items-center justify-between border-b border-input px-2 py-1">
            <code className="text-xs">{f.path}</code>
            <CopyButton text={f.content} />
          </div>
          <pre className="max-h-32 max-w-full overflow-auto p-2 text-xs">
            <code>{f.content}</code>
          </pre>
        </div>
      ))}
    </div>
  )
}

function BranchField({
  values,
  update,
  defaultBranch,
}: Readonly<{
  values: FlowValues
  update: (p: Partial<FlowValues>) => void
  defaultBranch: string
}>) {
  const { t } = useTranslation('functions')
  const url = useDebouncedValue(values.repoUrl.trim(), URL_DEBOUNCE_MS)
  const branches = useGitBranches(isHttpUrl(url) ? url : null)
  const list = useMemo(() => branches.data ?? [], [branches.data])
  useEffect(() => {
    if (list.length === 0 || list.includes(values.ref)) return
    const preferred = [defaultBranch, FALLBACK_BRANCH, 'master'].find((b) =>
      list.includes(b),
    )
    update({ ref: preferred ?? list[0] ?? FALLBACK_BRANCH })
  }, [list, values.ref, defaultBranch, update])

  return (
    <div className="space-y-1.5">
      <Label htmlFor="fn-ref">{t('deploy.source.ref')}</Label>
      {list.length > 0 ? (
        <Select
          value={values.ref}
          onValueChange={(v) => {
            update({ ref: v ?? defaultBranch })
          }}
        >
          <SelectTrigger id="fn-ref" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {list.map((b) => (
              <SelectItem key={b} value={b}>
                {b}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <Input
          id="fn-ref"
          value={values.ref}
          onChange={(e) => {
            update({ ref: e.target.value })
          }}
        />
      )}
      <FieldHelp>{t('deploy.source.refHint')}</FieldHelp>
    </div>
  )
}

function DetectionHint({
  values,
  onPort,
}: Readonly<{ values: FlowValues; onPort: (port: number) => void }>) {
  const { t } = useTranslation('functions')
  const detect = useDetectFramework()
  const { mutate } = detect
  const url = useDebouncedValue(values.repoUrl.trim(), URL_DEBOUNCE_MS)
  useEffect(() => {
    if (isHttpUrl(url)) mutate({ repoUrl: url, ref: values.ref })
  }, [url, values.ref, mutate])
  useEffect(() => {
    if (detect.data?.port) onPort(detect.data.port)
  }, [detect.data, onPort])
  if (detect.isPending) {
    return <FieldHelp>{t('deploy.source.detecting')}</FieldHelp>
  }
  if (!detect.data?.detected || !detect.data.frameworkName) return null
  return (
    <FieldHelp>
      {t('deploy.source.detected', { framework: detect.data.frameworkName })}
    </FieldHelp>
  )
}

export function StepSource({
  preset,
  values,
  errors,
  update,
  onDetectedPort,
}: Readonly<{
  preset: Preset
  values: FlowValues
  errors: FieldErrors
  update: (p: Partial<FlowValues>) => void
  onDetectedPort: (port: number) => void
}>) {
  const { t } = useTranslation('functions')
  const [paste, setPaste] = useState(false)
  const [defaultBranch, setDefaultBranch] = useState(FALLBACK_BRANCH)

  if (values.source === 'image') {
    return (
      <div className="min-w-0 space-y-4">
        <h3 className="text-base font-semibold">
          {t('deploy.source.headingImage')}
        </h3>
        <div className="space-y-1.5">
          <Label htmlFor="fn-image">{t('deploy.source.image')}</Label>
          <Input
            id="fn-image"
            value={values.image}
            placeholder={t('deploy.source.imagePlaceholder')}
            aria-invalid={errors.image ? true : undefined}
            aria-describedby="fn-image-error"
            onChange={(e) => {
              update({ image: e.target.value })
            }}
          />
          <FieldHelp>{t('deploy.source.imageHint')}</FieldHelp>
          <FieldError id="fn-image-error" messageKey={errors.image} />
        </div>
      </div>
    )
  }

  return (
    <div className="min-w-0 space-y-4">
      <h3 className="text-base font-semibold">
        {t('deploy.source.headingRepo')}
      </h3>
      {preset.runtime ? <StarterCode preset={preset} /> : null}
      {paste ? (
        <div className="space-y-1.5">
          <Label htmlFor="fn-repo">{t('deploy.source.repoUrl')}</Label>
          <Input
            id="fn-repo"
            inputMode="url"
            value={values.repoUrl}
            placeholder={t('deploy.source.repoUrlPlaceholder')}
            aria-invalid={errors.repoUrl ? true : undefined}
            aria-describedby="fn-repo-error"
            onChange={(e) => {
              update({ repoUrl: e.target.value })
            }}
          />
        </div>
      ) : (
        <RepoPicker
          selectedUrl={values.repoUrl}
          onPick={(repo: GitRepoItem) => {
            setDefaultBranch(repo.default_branch || FALLBACK_BRANCH)
            update({
              repoUrl: repo.clone_url,
              ref: repo.default_branch || FALLBACK_BRANCH,
            })
          }}
        />
      )}
      <FieldError id="fn-repo-error" messageKey={errors.repoUrl} />
      <button
        type="button"
        className="text-xs underline"
        onClick={() => {
          setPaste(!paste)
        }}
      >
        {paste
          ? t('deploy.source.pickInstead')
          : t('deploy.source.pasteInstead')}
      </button>
      {values.repoUrl.trim() !== '' ? (
        <div className="min-w-0 space-y-4">
          <BranchField
            values={values}
            update={update}
            defaultBranch={defaultBranch}
          />
          <div className="space-y-1.5">
            <Label htmlFor="fn-dir">{t('deploy.source.directory')}</Label>
            <Input
              id="fn-dir"
              value={values.directory}
              onChange={(e) => {
                update({ directory: e.target.value })
              }}
            />
            <FieldHelp>{t('deploy.source.directoryHint')}</FieldHelp>
            <DetectionHint values={values} onPort={onDetectedPort} />
          </div>
        </div>
      ) : null}
    </div>
  )
}
