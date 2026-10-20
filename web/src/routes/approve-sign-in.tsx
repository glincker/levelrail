import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '../components/shell/PageTemplate'
import { SignInApprovalCard } from '../components/signin/SignInApprovalCard'

export const Route = createFileRoute('/approve-sign-in')({
  component: ApproveSignInPage,
})

function ApproveSignInPage() {
  const { t } = useTranslation('signIn')
  return (
    <PageTemplate
      title={t('approvalCard.pageTitle')}
      description={t('approvalCard.pageDescription')}
    >
      <SignInApprovalCard showEmpty focusOnArrival />
    </PageTemplate>
  )
}
