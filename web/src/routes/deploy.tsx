import { useMemo, useState } from 'react'
import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { NewAppWizard } from '../components/newapp/NewAppWizard'
import { RepoLinkNotice } from '../components/newapp/RepoLinkNotice'
import { RepoLinkSummary } from '../components/newapp/RepoLinkSummary'
import { parseRepoLink } from '../lib/deployRepoLink'
import { stringifySearch } from '../lib/deployLink'
import { useServiceTemplates } from '../queries/serviceTemplates'

export const Route = createFileRoute('/deploy')({
  validateSearch: (search: Record<string, unknown>): Record<string, string> =>
    stringifySearch(search),
  component: DeployLinkPage,
})

/**
 * Landing page for README "Deploy to ..." buttons. It only prefills: the
 * analysis starts on a click and nothing is created before the wizard's own
 * final Deploy.
 */
function DeployLinkPage() {
  const { t } = useTranslation('newApp')
  const search = Route.useSearch()
  const templates = useServiceTemplates()
  const known = useMemo(
    () =>
      templates.data ? new Set(templates.data.map((x) => x.id)) : undefined,
    [templates.data],
  )
  const key = JSON.stringify(search)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const prefill = useMemo(() => parseRepoLink(search, known), [key, known])
  const [started, setStarted] = useState(false)
  const templateName = templates.data?.find(
    (x) => x.id === prefill.template,
  )?.name

  const usable = prefill.repo !== undefined || prefill.template !== undefined
  return (
    <PageTemplate
      title={t('deployPage.title')}
      description={t('deployPage.description')}
      width={started ? 'wide' : 'default'}
    >
      <div className="mx-auto w-full max-w-3xl space-y-5">
        {prefill.hasParams ? (
          <RepoLinkNotice ignored={prefill.ignored} />
        ) : null}
        {usable ? (
          <>
            {started ? null : (
              <>
                <RepoLinkSummary
                  prefill={prefill}
                  templateName={templateName}
                />
                {prefill.repo !== undefined ? (
                  <Button
                    onClick={() => {
                      setStarted(true)
                    }}
                  >
                    {t('deployPage.reviewRepo')}
                  </Button>
                ) : (
                  <Button
                    render={
                      <Link
                        to="/templates/$id"
                        params={{ id: prefill.template ?? '' }}
                      />
                    }
                    nativeButton={false}
                  >
                    {t('deployPage.reviewTemplate')}
                  </Button>
                )}
              </>
            )}
          </>
        ) : (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              {t('deployPage.empty')}
            </p>
            <Button render={<Link to="/apps/new" />} nativeButton={false}>
              {t('deployPage.startNew')}
            </Button>
          </div>
        )}
      </div>
      {started && prefill.repo !== undefined ? (
        <NewAppWizard
          initial={{
            source: {
              provider: 'manual',
              repoUrl: prefill.repo,
              branch: prefill.branch ?? '',
            },
            name: prefill.name,
            port: prefill.port,
          }}
        />
      ) : null}
    </PageTemplate>
  )
}
