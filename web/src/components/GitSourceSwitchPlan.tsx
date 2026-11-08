import { useTranslation } from 'react-i18next'
import { LoaderGlassIcon, ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useGitSourcePlan } from '../queries/gitSourceConnect'
import type { GitSourcePlanSuggestion } from '../queries/gitSourceConnect'
import { useDebouncedValue } from '../hooks/useDebouncedValue'

// Typing a repo URL must not send one plan request per keystroke.
const PLAN_DEBOUNCE_MS = 400

// Dry run of attaching a repo to an app that already runs from an image or
// an upload: what stays, what the first build will use.
export function GitSourceSwitchPlan({
  appName,
  repoUrl,
  branch,
  onApply,
}: {
  appName: string
  repoUrl: string
  branch: string
  onApply: (s: GitSourcePlanSuggestion) => void
}) {
  const { t } = useTranslation('gitSource')
  const repo = useDebouncedValue(repoUrl.trim(), PLAN_DEBOUNCE_MS)
  const debouncedBranch = useDebouncedValue(branch.trim(), PLAN_DEBOUNCE_MS)
  const plan = useGitSourcePlan(appName, repo, debouncedBranch, true)

  if (plan.isLoading) {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <LoaderGlassIcon className="size-4 animate-spin" />
        {t('connect.planLoading')}
      </p>
    )
  }
  if (!plan.data) return null
  const p = plan.data
  const s = p.suggestion

  return (
    <section
      aria-label={t('connect.planTitle')}
      className="space-y-3 rounded-2xl border border-border p-4"
    >
      <div className="flex items-start gap-2">
        <ShieldCheckGlassIcon className="mt-0.5 size-5 shrink-0 text-tone-success" />
        <div className="space-y-1">
          <p className="text-sm font-medium">
            {t(`connect.current.${p.current_source}`)}
          </p>
          <p className="text-sm text-muted-foreground">
            {p.preserved.running_image
              ? t('connect.keepsImage', { image: p.preserved.running_image })
              : t('connect.keepsNothingRunning')}
          </p>
        </div>
      </div>
      <ul className="grid gap-1 text-sm text-muted-foreground sm:grid-cols-2">
        <li>{t('connect.keptEnv', { count: p.preserved.env_vars })}</li>
        <li>{t('connect.keptSecrets', { count: p.preserved.secret_env })}</li>
        <li>
          {t('connect.keptDomains', { count: p.preserved.domains.length })}
        </li>
        <li>
          {t('connect.keptVolumes', { count: p.preserved.volumes.length })}
        </li>
      </ul>
      {p.warnings.map((w) => (
        <p key={w} className="text-sm text-tone-warning">
          {t(`connect.warning.${w}`, { defaultValue: w })}
        </p>
      ))}
      {p.detect_error ? (
        <p className="text-sm text-tone-warning">
          {t(`connect.detectError.${p.detect_error}`)}
        </p>
      ) : null}
      {s ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-sm">
            {t('connect.suggested', {
              build: t(`connect.buildType.${s.build_type}`),
              where: s.base_directory || s.build_path || '/',
            })}
            {s.build_command
              ? ` ${s.build_command} -> ${s.publish_dir ?? ''}`
              : ''}
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => onApply(s)}
          >
            {t('connect.useSuggestion')}
          </Button>
        </div>
      ) : repo ? null : (
        <p className="text-sm text-muted-foreground">
          {t('connect.pickRepoToDetect')}
        </p>
      )}
    </section>
  )
}
