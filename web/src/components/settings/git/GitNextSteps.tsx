import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'

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
    <section aria-labelledby="git-next-steps" className="space-y-3">
      <h2 id="git-next-steps" className="text-sm font-medium">
        {t('next.title')}
      </h2>
      <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {STEPS.map((step) => (
          <li key={step.id} className="flex">
            <Card size="sm" className="w-full justify-between">
              <div className="space-y-1 px-(--card-spacing)">
                <h3 className="text-sm font-medium">
                  {t(`next.${step.id}.title`)}
                </h3>
                <p className="text-xs text-muted-foreground">
                  {t(`next.${step.id}.body`)}
                </p>
              </div>
              <div className="px-(--card-spacing)">
                <Button size="sm" variant="outline" render={step.link}>
                  {t(`next.${step.id}.action`)}
                </Button>
              </div>
            </Card>
          </li>
        ))}
      </ul>
    </section>
  )
}
