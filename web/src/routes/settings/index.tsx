import { createFileRoute, Link } from '@tanstack/react-router'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { visibleSettingsSections } from '../../lib/settingsNav'
import { PageHeader } from '../../components/shell/PageHeader'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
} from '../../components/ui/card'

export const Route = createFileRoute('/settings/')({
  component: SettingsHubPage,
})

function SettingsHubPage() {
  const experimental = useExperimentalFeatures()
  return (
    <div className="space-y-8">
      <PageHeader
        title="Settings"
        description="Account, team, and platform configuration."
      />

      {visibleSettingsSections(experimental).map((section) => (
        <div key={section.heading} className="space-y-3">
          <h2 className="text-sm font-medium text-muted-foreground">
            {section.heading}
          </h2>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {section.items.map((item) => (
              <Link key={item.to} to={item.to} className="block">
                <Card className="h-full rounded-[var(--glinui-radius-lg)] transition-colors hover:ring-foreground/20">
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <item.icon className="size-4" />
                      {item.title}
                    </CardTitle>
                    <CardDescription>{item.description}</CardDescription>
                  </CardHeader>
                </Card>
              </Link>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
