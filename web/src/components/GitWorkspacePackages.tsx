import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LoaderGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { useCheckGitSourceAffected } from '../queries/gitBuildSettings'
import type { WorkspacePackage } from '../types/gitSource'

interface GitWorkspacePackagesProps {
  appName: string
  packages: WorkspacePackage[]
  applying: boolean
  onUse: (pkg: WorkspacePackage) => void
}

// GitWorkspacePackages lists the workspace members found in the repository,
// applies one as this app's root with its watch paths, and dry-runs a change.
export function GitWorkspacePackages({
  appName,
  packages,
  applying,
  onUse,
}: GitWorkspacePackagesProps) {
  const { t } = useTranslation('gitBuild')
  const [files, setFiles] = useState('')
  const check = useCheckGitSourceAffected(appName)

  if (packages.length === 0) return null

  function runCheck() {
    const list = files
      .split('\n')
      .map((f) => f.trim())
      .filter(Boolean)
    if (list.length > 0) check.mutate(list)
  }

  const verdict = check.data

  return (
    <section
      aria-label={t('workspace.title')}
      className="space-y-2 rounded-lg border border-input bg-muted/30 p-3"
    >
      <div>
        <h4 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          {t('workspace.title')}
        </h4>
        <p className="text-xs text-muted-foreground">
          {t('workspace.description')}
        </p>
      </div>

      <ul className="space-y-2">
        {packages.map((p) => (
          <li
            key={p.dir}
            className="flex flex-wrap items-start justify-between gap-3 rounded-md border border-input bg-background p-2.5"
          >
            <div className="min-w-0 flex-1 space-y-1">
              <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                <span className="font-mono break-all">{p.dir}</span>
                <Badge variant="muted">
                  {t('workspace.tool', { tool: p.tool })}
                </Badge>
                {verdict ? (
                  <Badge
                    variant={
                      verdict.packages.find((v) => v.dir === p.dir)?.affected
                        ? 'default'
                        : 'outline'
                    }
                  >
                    {verdict.packages.find((v) => v.dir === p.dir)?.affected
                      ? t('workspace.affected')
                      : t('workspace.unaffected')}
                  </Badge>
                ) : null}
              </p>
              <p className="text-xs break-all text-muted-foreground">
                {t('workspace.watching', { paths: p.watch_paths.join(', ') })}
              </p>
              {p.depends_on.length > 0 ? (
                <p className="text-xs text-muted-foreground">
                  {t('workspace.dependsOn', { deps: p.depends_on.join(', ') })}
                </p>
              ) : null}
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={applying}
              onClick={() => {
                onUse(p)
              }}
            >
              {t('workspace.use')}
            </Button>
          </li>
        ))}
      </ul>

      <div className="space-y-2">
        <label
          htmlFor="workspace-changed-files"
          className="text-xs font-medium text-muted-foreground"
        >
          {t('workspace.checkLabel')}
        </label>
        <Textarea
          id="workspace-changed-files"
          value={files}
          placeholder={t('workspace.checkPlaceholder')}
          onChange={(e) => {
            setFiles(e.target.value)
          }}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={check.isPending || files.trim() === ''}
          onClick={runCheck}
        >
          {check.isPending ? (
            <LoaderGlassIcon className="size-4.5 animate-spin" />
          ) : null}
          {check.isPending ? t('workspace.checking') : t('workspace.check')}
        </Button>
        {verdict ? (
          <p className="text-xs text-muted-foreground" role="status">
            {t('workspace.thisApp', {
              verdict: verdict.app_deploys
                ? t('workspace.appDeploys')
                : t('workspace.appSkipped', { reason: verdict.reason ?? '' }),
            })}
          </p>
        ) : null}
        {check.error ? (
          <p className="text-xs text-destructive" role="alert">
            {check.error.message}
          </p>
        ) : null}
      </div>
    </section>
  )
}
