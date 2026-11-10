import { useContext } from 'react'
import { useTranslation } from 'react-i18next'
import { CodeGlassIcon, TerminalGlassIcon } from '@thesvg/ui-react'
import { ActionMenu, type ActionMenuItem } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import { BrandContext } from '../lib/brandContext'
import {
  cliCommand,
  curlCommand,
  type CommandAction,
  type CommandKind,
} from '../lib/cliCommands'

type KindKey = {
  [K in CommandKind]: K extends `${infer A}.${infer B}` ? `${A}_${B}` : K
}[CommandKind]

function currentOrigin(): string {
  return typeof window === 'undefined' ? '' : window.location.origin
}

/** CopyAsCommand offers the CLI command and API call behind each row action. */
export function CopyAsCommand({
  actions,
  subject,
  className,
}: {
  actions: CommandAction[]
  subject: string
  className?: string
}) {
  const { t } = useTranslation('common')
  // Optional by design: a row rendered outside the brand provider (isolated
  // previews, tests) simply has no copy menu instead of crashing the row.
  const brand = useContext(BrandContext)
  const { copy } = useCopyToClipboard()
  if (!brand || actions.length === 0) return null

  const copyWithToast = (value: string, title: string) => {
    copy(value)
    toast.add({ title, type: 'success' })
  }
  const origin = currentOrigin()
  const items: ActionMenuItem[] = actions.flatMap((action) => {
    const name = t(
      `copyCommand.actions.${action.kind.replace('.', '_') as KindKey}`,
    )
    const cli = cliCommand(action, brand.BinaryName)
    const curl = curlCommand(action, brand.BinaryName, origin)
    const rows: ActionMenuItem[] = [
      {
        id: `${action.kind}-cli`,
        label: t('copyCommand.cli', { action: name }),
        icon: <TerminalGlassIcon size="1.4em" aria-hidden="true" />,
        onSelect: () => {
          copyWithToast(cli, t('copyCommand.copiedCli'))
        },
      },
    ]
    if (curl) {
      rows.push({
        id: `${action.kind}-api`,
        label: t('copyCommand.api', { action: name }),
        icon: <CodeGlassIcon size="1.4em" aria-hidden="true" />,
        onSelect: () => {
          copyWithToast(curl, t('copyCommand.copiedApi'))
        },
      })
    }
    return rows
  })

  return (
    <ActionMenu
      trigger={
        <Button
          variant="ghost"
          size="icon-sm"
          className={className}
          aria-label={t('copyCommand.trigger', { name: subject })}
          title={t('copyCommand.title')}
        >
          <TerminalGlassIcon size="1.4em" aria-hidden="true" />
        </Button>
      }
      items={items}
    />
  )
}
