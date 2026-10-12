import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowRightIcon } from '@phosphor-icons/react/dist/ssr'
import { CreateResourceWizard } from '../CreateResourceWizard'

const ROW =
  'group flex w-full items-center gap-3 px-4 py-3 text-left transition-colors duration-200 hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-ring'

function Body({ title, hint }: { title: string; hint: string }) {
  return (
    <>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-semibold text-foreground">
          {title}
        </span>
        <span className="block text-xs text-muted-foreground">{hint}</span>
      </span>
      <ArrowRightIcon
        aria-hidden="true"
        className="size-4 shrink-0 text-muted-foreground transition-transform duration-200 group-hover:translate-x-0.5 motion-reduce:transition-none"
      />
    </>
  )
}

/** QuickStart is the first-run list: three ways to get something deployed. */
export function QuickStart() {
  const { t } = useTranslation('dashboard')
  return (
    <section aria-label={t('quickStart.title')} className="space-y-3">
      <h2 className="text-sm font-semibold text-foreground">
        {t('quickStart.title')}
      </h2>
      <div className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-card">
        <Link to="/settings/import-platform" className={ROW}>
          <Body
            title={t('quickStart.import.title')}
            hint={t('quickStart.import.hint')}
          />
        </Link>
        <CreateResourceWizard
          initialSelected="browse-templates"
          trigger={
            <button type="button" className={ROW}>
              <Body
                title={t('quickStart.template.title')}
                hint={t('quickStart.template.hint')}
              />
            </button>
          }
        />
        <Link to="/settings/github-app" className={ROW}>
          <Body
            title={t('quickStart.git.title')}
            hint={t('quickStart.git.hint')}
          />
        </Link>
      </div>
    </section>
  )
}
