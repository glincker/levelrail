import { isMailUp, useMailStatus } from '../../queries/mailStatus'
import { useMailDomains } from '../../queries/mailMailboxes'
import { MailBackupPanel } from './MailBackupPanel'
import { MailStoragePanel } from './MailStoragePanel'
import { MailAliasesPanel } from './MailAliasesPanel'
import { MailMailboxesPanel } from './MailMailboxesPanel'
import { MailQueuePanel } from './MailQueuePanel'

/** Working mail panels; rendered only once the mail service is installed and healthy. */
export function MailPanels() {
  const status = useMailStatus().data
  const up = isMailUp(status)
  const domains = useMailDomains(up).data ?? []
  if (!up) return null
  return (
    <div className="space-y-3">
      <MailMailboxesPanel domains={domains} />
      <MailAliasesPanel domains={domains} />
      <MailQueuePanel />
      {status?.app && <MailStoragePanel app={status.app} />}
      {status?.app && <MailBackupPanel app={status.app} />}
    </div>
  )
}
