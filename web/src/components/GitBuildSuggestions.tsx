import { useTranslation } from 'react-i18next'
import { SpinnerIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { GitBuildDetection, GitBuildSuggestion } from '../types/gitSource'

interface GitBuildSuggestionsProps {
  branch: string
  detection?: GitBuildDetection
  loading: boolean
  error?: Error | null
  applying: boolean
  onRetry: () => void
  onApply: (suggestion: GitBuildSuggestion) => void
}

function dirName(path: string): string {
  const i = path.lastIndexOf('/')
  return i < 0 ? '' : path.slice(0, i)
}

function reasonParams(s: GitBuildSuggestion): Record<string, string> {
  const dockerfileDir = dirName(s.dockerfile_path ?? '')
  return {
    dockerfile: s.dockerfile_path ?? '',
    app: dirName(dockerfileDir) || dockerfileDir,
    directory: s.base_directory ?? dockerfileDir,
  }
}

// GitBuildSuggestions lists what detection found in the repository, each with
// the plain reason and a one-click apply.
export function GitBuildSuggestions({
  branch,
  detection,
  loading,
  error,
  applying,
  onRetry,
  onApply,
}: GitBuildSuggestionsProps) {
  const { t } = useTranslation('gitBuild')

  function reasonText(s: GitBuildSuggestion): string {
    const p = reasonParams(s)
    switch (s.reason_code) {
      case 'turbo_prune':
        return t('reasons.turboPrune')
      case 'root_copy':
        return t('reasons.rootCopy')
      case 'app_deploy_dir':
        return t('reasons.appDeployDir', { app: p.app })
      case 'root_dockerfile':
        return t('reasons.rootDockerfile')
      case 'nested_dockerfile':
        return t('reasons.nestedDockerfile', { directory: p.directory })
      case 'app_railpack':
        return t('reasons.appRailpack', { directory: p.directory })
      default:
        return s.reason
    }
  }

  return (
    <section
      aria-label={t('detected.title')}
      className="space-y-2 rounded-lg border border-input bg-muted/30 p-3"
    >
      <div>
        <h4 className="text-sm font-medium">{t('detected.title')}</h4>
        <p className="text-xs text-muted-foreground">
          {t('detected.description', { branch })}
        </p>
      </div>

      {loading ? (
        <p
          className="flex items-center gap-2 text-sm text-muted-foreground"
          role="status"
        >
          <SpinnerIcon className="size-4 animate-spin" />
          {t('detected.loading')}
        </p>
      ) : null}

      {error ? (
        <div className="flex items-start gap-2 text-destructive" role="alert">
          <WarningIcon className="mt-0.5 size-4 shrink-0" />
          <div className="space-y-1">
            <p className="text-sm">
              {t('detected.error', { error: error.message })}
            </p>
            <Button type="button" size="sm" variant="outline" onClick={onRetry}>
              {t('detected.retry')}
            </Button>
          </div>
        </div>
      ) : null}

      {detection && detection.suggestions.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('detected.empty')}</p>
      ) : null}

      {detection && detection.suggestions.length > 0 ? (
        <ul className="space-y-2">
          {detection.suggestions.map((s) => {
            const key = `${s.build_type}:${s.dockerfile_path ?? ''}:${s.base_directory ?? ''}`
            const context = s.base_directory || t('values.repoRoot')
            return (
              <li
                key={key}
                className="flex flex-wrap items-start justify-between gap-3 rounded-md border border-input bg-background p-2.5"
              >
                <div className="min-w-0 flex-1 space-y-1">
                  <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                    <span className="font-mono break-all">
                      {s.dockerfile_path
                        ? t('detected.dockerfileLine', {
                            path: s.dockerfile_path,
                          })
                        : t('detected.autoDetectLine', { where: context })}
                    </span>
                    {s.recommended ? (
                      <Badge variant="muted">{t('detected.recommended')}</Badge>
                    ) : null}
                  </p>
                  {s.dockerfile_path ? (
                    <p className="text-xs text-muted-foreground">
                      {t('detected.contextLine', { where: context })}
                    </p>
                  ) : null}
                  <p className="text-xs text-muted-foreground">
                    {reasonText(s)}
                  </p>
                </div>
                <Button
                  type="button"
                  size="sm"
                  variant={s.recommended ? 'default' : 'outline'}
                  disabled={applying}
                  onClick={() => {
                    onApply(s)
                  }}
                >
                  {t('detected.apply')}
                </Button>
              </li>
            )
          })}
        </ul>
      ) : null}

      {detection && detection.tools.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('detected.tools', { tools: detection.tools.join(', ') })}
        </p>
      ) : null}
      {detection?.truncated ? (
        <p className="text-xs text-muted-foreground">
          {t('detected.truncated')}
        </p>
      ) : null}
    </section>
  )
}
