import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GitBranchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useGitSource } from '../../queries/gitSources'
import { useDeployGitSource } from '../../queries/gitSourceConnect'
import { usePagesSiteDetail } from '../../queries/pagesSites'
import { usePreviewEnvironments } from '../../queries/previewEnvironments'

function Row({
  label,
  children,
}: Readonly<{ label: string; children: React.ReactNode }>) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="min-w-0 truncate text-sm">{children}</dd>
    </div>
  )
}

/** Repo, branch, last commit, trigger state and branch previews for a site deployed from git. Renders nothing for upload sites. */
export default function SiteGitCard({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const source = useGitSource(name)
  const detail = usePagesSiteDetail(name)
  const previews = usePreviewEnvironments(name)
  const redeploy = useDeployGitSource(name)
  const gs = source.data
  if (!gs) return null
  const sha = detail.data?.latest_deploy?.commit_sha
  const live = (previews.data ?? []).filter(
    (p) => p.domain && p.status === 'active',
  )
  return (
    <section
      aria-labelledby="pages-git-card"
      className="space-y-3 rounded-lg border border-border bg-card p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2
          id="pages-git-card"
          className="flex items-center gap-2 text-sm font-medium"
        >
          <GitBranchGlassIcon className="size-4" aria-hidden="true" />
          {t('git.card.title')}
        </h2>
        <span className="flex gap-2">
          <Button
            size="sm"
            variant="ghost"
            render={<Link to="/apps/$name" params={{ name }} />}
          >
            {t('git.card.openApp')}
          </Button>
          <Button
            size="sm"
            disabled={redeploy.isPending}
            onClick={() =>
              redeploy.mutate(undefined, {
                onSuccess: () =>
                  toast.add({ title: t('git.card.redeployStarted') }),
                onError: (err) =>
                  toast.add({
                    title: t('git.card.redeployFailed', {
                      message: err.message,
                    }),
                  }),
              })
            }
          >
            {redeploy.isPending
              ? t('git.card.redeploying')
              : t('git.card.redeploy')}
          </Button>
        </span>
      </div>
      <dl className="grid gap-3 min-[480px]:grid-cols-2 lg:grid-cols-4">
        <Row label={t('git.card.repo')}>
          <span className="font-mono text-xs" title={gs.repo_url}>
            {gs.repo_url.replace(/^https?:\/\//, '').replace(/\.git$/, '')}
          </span>
        </Row>
        <Row label={t('git.card.branch')}>{gs.branch}</Row>
        <Row label={t('git.card.commit')}>
          {sha ? (
            <span className="font-mono text-xs">{sha.slice(0, 7)}</span>
          ) : (
            t('git.card.none')
          )}
        </Row>
        <Row label={t('git.card.onPush')}>
          {gs.trigger_mode === 'release'
            ? t('git.card.onPushRelease')
            : t('git.card.onPushOn', { branch: gs.branch })}
        </Row>
      </dl>
      <p className="text-xs text-muted-foreground">
        {t('git.card.onPushHint')}
      </p>
      {live.length > 0 ? (
        <div className="space-y-1.5">
          <h3 className="text-xs text-muted-foreground">
            {t('git.card.previews')}
          </h3>
          <ul className="flex flex-wrap gap-2">
            {live.map((p) => (
              <li key={p.preview_app_id}>
                <a
                  href={`https://${p.domain}`}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1.5 rounded-full border border-border px-2.5 py-0.5 text-xs hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
                  title={`${t('git.card.previewOpen')}: ${p.domain}`}
                >
                  <span>{t('git.card.pr', { number: p.pr_number })}</span>
                  <span className="font-mono">{p.branch}</span>
                </a>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </section>
  )
}
