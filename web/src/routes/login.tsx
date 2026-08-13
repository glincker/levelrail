import { createFileRoute, redirect } from '@tanstack/react-router'
import { useState } from 'react'
import { getStoredUsername } from '../lib/authStore'
import { brandQueryOptions } from '../queries/brand'
import { useBrand } from '../hooks/useBrand'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../components/ui/tabs'
import { LoginForm } from '../components/LoginForm'
import { RegisterForm } from '../components/RegisterForm'

type LoginTab = 'sign-in' | 'register'

// The one login route, handling both the returning-operator and the
// first-run cases (docs-local/research/dashboard-gap-audit-and-devmode.md
// gap #1 and #3). There is deliberately no "does an admin already exist"
// signal from the backend, so this does not auto-detect first-run and
// render a different screen for it; it's one screen with a tab toggle the
// operator picks themselves, a real, intentional deviation from
// auto-detection, not an oversight (see TASKS.md's note on this task for
// the full reasoning).
//
// Already-authenticated visitors get bounced to /apps: this route's own
// job is only ever reached when there's no session to speak of.
export const Route = createFileRoute('/login')({
  beforeLoad: () => {
    if (getStoredUsername() !== null) {
      redirect({ to: '/apps', throw: true })
    }
  },
  // The login screen needs branding before a session exists (CLAUDE.md
  // section 3), same as the root route's own loader; primed again here so
  // this route works even if it's the very first thing to load (root's
  // beforeLoad redirect can land here before root's own loader runs on a
  // fresh navigation, so this route can't assume the cache is warm yet).
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(brandQueryOptions()),
  component: LoginPage,
})

function LoginPage() {
  const brand = useBrand()
  const [tab, setTab] = useState<LoginTab>('sign-in')

  function handleTabChange(value: unknown): void {
    if (value === 'sign-in' || value === 'register') {
      setTab(value)
    }
  }

  return (
    <div className="flex min-h-[70vh] items-center justify-center">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Sign in to {brand.ShortName || brand.Name}</CardTitle>
          <CardDescription>
            Enter your admin credentials, or set up the admin account on first
            run.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs value={tab} onValueChange={handleTabChange}>
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="sign-in">Sign in</TabsTrigger>
              <TabsTrigger value="register">Set up admin account</TabsTrigger>
            </TabsList>
            <TabsContent value="sign-in">
              <LoginForm />
            </TabsContent>
            <TabsContent value="register">
              <RegisterForm onSwitchToSignIn={() => setTab('sign-in')} />
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </div>
  )
}
