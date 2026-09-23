import { createFileRoute } from '@tanstack/react-router'
import { onboardingQueryOptions } from '../../queries/onboarding'
import { userListQueryOptions } from '../../queries/users'
import { SetupWizard } from '../../components/setup/SetupWizard'
import { useIsRoot } from '../../hooks/useIsRoot'
import { ListSkeleton } from '@/components/ui/list-skeleton'

export const Route = createFileRoute('/settings/setup-wizard')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(onboardingQueryOptions()),
      queryClient.prefetchQuery(userListQueryOptions()),
    ]),
  component: SetupWizardPage,
  pendingComponent: () => <ListSkeleton rows={5} />,
})

function SetupWizardPage() {
  const isRoot = useIsRoot()
  if (!isRoot) {
    return (
      <p className="text-sm text-muted-foreground">
        Only an administrator can run the setup wizard.
      </p>
    )
  }
  return <SetupWizard />
}
