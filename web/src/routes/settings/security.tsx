import { createFileRoute } from '@tanstack/react-router'

// Placeholder shell, same reasoning as routes/settings/account.tsx.
export const Route = createFileRoute('/settings/security')({
  component: SecuritySettingsPage,
})

function SecuritySettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Security</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Sessions and login protection.
        </p>
      </div>
    </div>
  )
}
