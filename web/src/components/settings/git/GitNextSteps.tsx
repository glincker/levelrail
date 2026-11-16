import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CardGrid, ObjectCard } from '../../cards/ObjectCard'

interface Step {
  id: 'import' | 'previews' | 'ci' | 'webhook' | 'registry'
  link: React.ReactElement
}

// Literal routes per step keep the router's link typing cheap to check.
const STEPS: readonly Step[] = [
  { id: 'import', link: <Link to="/sources" /> },
  { id: 'previews', link: <Link to="/sources" /> },
  { id: 'ci', link: <Link to="/pipelines" /> },
  { id: 'webhook', link: <Link to="/settings/event-webhooks" /> },
  { id: 'registry', link: <Link to="/settings/registry-credentials" /> },
]

// Next steps after connecting, each a deep link to a page that exists.
export function GitNextSteps() {
  const { t } = useTranslation('gitConnect')
  return (
    <section aria-labelledby="git-next-steps" className="space-y-4">
      <h2
        id="git-next-steps"
        className="font-(family-name:--glinr-font-display) text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)"
      >
        {t('next.title')}
      </h2>
      <CardGrid min="sm">
        {STEPS.map((step) => (
          <li key={step.id} className="flex min-w-0">
            <ObjectCard
              title={t(`next.${step.id}.title`)}
              actions={
                <Button size="sm" variant="outline" render={step.link}>
                  {t(`next.${step.id}.action`)}
                </Button>
              }
            >
              <p className="text-[13px] text-muted-foreground">
                {t(`next.${step.id}.body`)}
              </p>
            </ObjectCard>
          </li>
        ))}
      </CardGrid>
    </section>
  )
}
