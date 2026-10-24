import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  ArrowsClockwiseIcon,
  GearSixIcon,
  GitBranchIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { BrandTile } from '../BrandLogoBadge'
import { useRefreshGitRepos } from '../../queries/gitRepos'
import type { ConnectedRepos } from '../../hooks/useConnectedRepos'
import type { GitProviderName } from '../../types/gitProviders'
import type { WorkspacePackage } from '../../queries/importAnalyze'

const PROVIDER_LABEL: Record<GitProviderName, string> = {
  github: 'GitHub',
  gitlab: 'GitLab',
  bitbucket: 'Bitbucket',
  bitbucket_server: 'Bitbucket Server',
  gitea: 'Gitea',
  forgejo: 'Forgejo',
}

function brandOf(p: GitProviderName): string {
  if (p === 'bitbucket_server') return 'bitbucket'
  if (p === 'forgejo') return 'gitea'
  return p
}

/** What the chosen repository lets the operator preset before the first deploy. */
export interface ImportDefaults {
  branch: string
  branches: string[]
  onBranch: (branch: string) => void
  packages: WorkspacePackage[]
  baseDirectory: string
  onBaseDirectory: (dir: string) => void
  deployOnPush: boolean
  onDeployOnPush: (on: boolean) => void
}

/**
 * Compact popover beside the repository field: connected accounts, a forced
 * refresh, the way to Settings, and the defaults the chosen repo can preset.
 */
export function GitSettings({
  accounts,
  defaults,
  open,
  onOpenChange,
}: {
  accounts: ConnectedRepos
  defaults: ImportDefaults | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('newApp')
  const refresh = useRefreshGitRepos('')
  const connected = accounts.providers.filter((p) => p.connected)

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger
        render={<Button type="button" size="sm" variant="outline" />}
      >
        <GearSixIcon aria-hidden="true" />
        {t('gitSettings.open')}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 gap-3 p-3">
        <section aria-label={t('gitSettings.accounts')} className="space-y-2">
          <h3 className="text-xs font-medium text-muted-foreground uppercase">
            {t('gitSettings.accounts')}
          </h3>
          {connected.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('gitSettings.none')}
            </p>
          ) : (
            <ul className="space-y-1.5">
              {connected.map((p) => {
                const rows = accounts.connections.filter(
                  (c) => c.provider === p.provider,
                )
                return (
                  <li key={p.provider} className="flex items-start gap-2">
                    <BrandTile
                      name={brandOf(p.provider)}
                      label={PROVIDER_LABEL[p.provider]}
                      size="xs"
                    />
                    <span className="min-w-0 text-sm">
                      <span className="font-medium text-foreground">
                        {PROVIDER_LABEL[p.provider]}
                      </span>
                      {rows.length === 0 ? (
                        <span className="block text-xs text-muted-foreground">
                          {t('source.git.connected')}
                        </span>
                      ) : (
                        rows.map((c) => (
                          <span
                            key={c.connection_id}
                            className="block truncate text-xs text-muted-foreground"
                          >
                            {c.account || c.label}
                            {c.failed
                              ? ` (${t('gitSettings.failed')})`
                              : ` (${t('gitSettings.loaded', { count: c.loaded })})`}
                          </span>
                        ))
                      )}
                    </span>
                  </li>
                )
              })}
            </ul>
          )}
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={refresh.isPending || connected.length === 0}
              onClick={() => {
                refresh.mutate()
              }}
            >
              <ArrowsClockwiseIcon aria-hidden="true" />
              {refresh.isPending
                ? t('source.repos.refreshing')
                : t('source.repos.refresh')}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              render={<Link to="/settings/git-connections" />}
              nativeButton={false}
            >
              {t('source.git.manage')}
            </Button>
          </div>
          {refresh.isError ? (
            <p role="alert" className="text-xs text-tone-danger">
              {t('gitSettings.refreshFailed', {
                reason: refresh.error.message,
              })}
            </p>
          ) : null}
        </section>

        <section
          aria-label={t('gitSettings.defaults')}
          className="space-y-2.5 border-t border-border pt-3"
        >
          <h3 className="text-xs font-medium text-muted-foreground uppercase">
            {t('gitSettings.defaults')}
          </h3>
          {defaults === null ? (
            <p className="text-sm text-muted-foreground">
              {t('gitSettings.pickFirst')}
            </p>
          ) : (
            <>
              <div className="flex items-center gap-2">
                <GitBranchIcon
                  aria-hidden="true"
                  className="size-4 shrink-0 text-muted-foreground"
                />
                <Select
                  value={defaults.branch}
                  onValueChange={(v) => {
                    if (v) defaults.onBranch(v)
                  }}
                >
                  <SelectTrigger
                    className="w-full min-w-0"
                    aria-label={t('import.card.branch')}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(defaults.branches.includes(defaults.branch)
                      ? defaults.branches
                      : [defaults.branch, ...defaults.branches]
                    ).map((b) => (
                      <SelectItem key={b} value={b}>
                        {b}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {defaults.packages.length > 1 ? (
                <Select
                  value={
                    defaults.baseDirectory || (defaults.packages[0]?.dir ?? '')
                  }
                  onValueChange={(v) => {
                    if (v) defaults.onBaseDirectory(v)
                  }}
                >
                  <SelectTrigger
                    className="w-full min-w-0"
                    aria-label={t('gitSettings.directory')}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {defaults.packages.map((p) => (
                      <SelectItem key={p.dir} value={p.dir}>
                        {p.name} ({p.dir})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : null}
              <div className="flex items-start justify-between gap-3">
                <label
                  htmlFor="git-settings-push"
                  className="min-w-0 text-sm text-foreground"
                >
                  {t('deployOptions.push.label', { branch: defaults.branch })}
                  <span className="block text-xs text-muted-foreground">
                    {defaults.deployOnPush
                      ? t('gitSettings.pushOn')
                      : t('deployOptions.push.off')}
                  </span>
                </label>
                <Switch
                  id="git-settings-push"
                  checked={defaults.deployOnPush}
                  onCheckedChange={defaults.onDeployOnPush}
                />
              </div>
            </>
          )}
        </section>
      </PopoverContent>
    </Popover>
  )
}
