import { createFileRoute } from '@tanstack/react-router'

// Placeholder shell, same reasoning as routes/settings/account.tsx.
export const Route = createFileRoute('/settings/general')({
  component: GeneralSettingsPage,
})

function GeneralSettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">General</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          System status and configuration.
        </p>
      </div>
    </div>
  )
}
