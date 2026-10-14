import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { requireExperimental } from '../lib/experimental'
import { CliConsole } from '../components/cliConsole/CliConsole'
import { DemoGate } from '@/components/demo/DemoUnavailable'
import { PageHeader } from '@/components/shell/PageHeader'

export const Route = createFileRoute('/terminal')({
  beforeLoad: ({ context: { queryClient } }) =>
    requireExperimental(queryClient, 'cli-console'),
  component: TerminalPage,
})

function TerminalPage() {
  const { t } = useTranslation()
  return (
    <div className="flex h-[calc(100vh-8rem)] flex-col gap-4">
      <PageHeader
        title={t('cliConsole.title')}
        description={t('cliConsole.description')}
      />
      <DemoGate reason="terminal">
        <CliConsole className="flex-1" />
      </DemoGate>
    </div>
  )
}
