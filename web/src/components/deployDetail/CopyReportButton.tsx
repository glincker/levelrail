import { useState } from 'react'
import { CheckIcon, ClipboardTextIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  buildDeployReport,
  type DeployReportInput,
} from '../../lib/deployReport'
import { useDeployDetailT } from './useDeployDetailT'

const COPIED_RESET_MS = 1500

/** CopyReportButton copies a redacted Markdown report; the text is built on click so it is always current. */
export function CopyReportButton({
  getInput,
}: {
  getInput: () => DeployReportInput
}) {
  const { t } = useDeployDetailT()
  const [copied, setCopied] = useState(false)

  const copy = () => {
    void navigator.clipboard
      .writeText(buildDeployReport(getInput()))
      .then(() => {
        setCopied(true)
        window.setTimeout(() => {
          setCopied(false)
        }, COPIED_RESET_MS)
      })
      .catch(() => {
        toast.add({ title: t('report.failed'), type: 'error' })
      })
  }

  return (
    <Button type="button" variant="outline" size="sm" onClick={copy}>
      {copied ? (
        <CheckIcon aria-hidden="true" />
      ) : (
        <ClipboardTextIcon aria-hidden="true" />
      )}
      {copied ? t('report.copied') : t('report.copy')}
    </Button>
  )
}
