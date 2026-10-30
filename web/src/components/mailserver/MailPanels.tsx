import { isMailUp, useMailStatus } from '../../queries/mailStatus'
import { useMailDomains } from '../../queries/mailMailboxes'
import { MailAliasesPanel } from './MailAliasesPanel'
import { MailMailboxesPanel } from './MailMailboxesPanel'
import { MailQueuePanel } from './MailQueuePanel'

/** Working mail panels; rendered only once the mail service is installed and healthy. */
export function MailPanels() {
  const up = isMailUp(useMailStatus().data)
  const domains = useMailDomains(up).data ?? []
  if (!up) return null
  return (
    <div className="space-y-3">
      <MailMailboxesPanel domains={domains} />
      <MailAliasesPanel domains={domains} />
      <MailQueuePanel />
    </div>
  )
}
