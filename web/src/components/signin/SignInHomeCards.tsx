import { ConnectCliCard } from './ConnectCliCard'
import { SignInApprovalCard } from './SignInApprovalCard'

export function SignInHomeCards() {
  return (
    <div className="space-y-4">
      <SignInApprovalCard />
      <ConnectCliCard />
    </div>
  )
}
