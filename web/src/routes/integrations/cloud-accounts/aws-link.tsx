import { createFileRoute } from '@tanstack/react-router'
import { AwsLinkWizard } from '../../../components/cloudAccounts/AwsLinkWizard'

export const Route = createFileRoute('/integrations/cloud-accounts/aws-link')({
  component: AwsLinkWizard,
})
