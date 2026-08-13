import { createFileRoute } from '@tanstack/react-router'

// Placeholder shell: real content (profile info, change-password form
// against PUT /api/v1/auth/password) is a follow-up pass, kept as its
// own file/route so that pass can build on real routing/nav wiring
// rather than needing to touch routes/__root.tsx or AppSidebar.tsx
// itself.
export const Route = createFileRoute('/settings/account')({
  component: AccountSettingsPage,
})

function AccountSettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Account</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Profile and password.
        </p>
      </div>
    </div>
  )
}
