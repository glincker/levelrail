import { useEffect, useRef, useState } from 'react'
import { CheckIcon, ClipboardTextIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  buildDeployReport,
  type DeployReportInput,
} from '../../lib/deployReport'
import { COPY_FEEDBACK_MS } from '../../lib/copyFeedback'
import { useDeployDetailT } from './useDeployDetailT'

/** CopyReportButton copies a redacted Markdown report; the text is built on click so it is always current. */
export function CopyReportButton({
  getInput,
}: {
  getInput: () => DeployReportInput
}) {
  const { t } = useDeployDetailT()
  const [copied, setCopied] = useState(false)
  const timer = useRef<number | undefined>(undefined)

  useEffect(() => () => window.clearTimeout(timer.current), [])

  const copy = () => {
    void navigator.clipboard
      .writeText(buildDeployReport(getInput()))
      .then(() => {
        setCopied(true)
        window.clearTimeout(timer.current)
        timer.current = window.setTimeout(() => {
          setCopied(false)
        }, COPY_FEEDBACK_MS)
      })
      .catch(() => {
        toast.add({ title: t('report.failed'), type: 'error' })
      })
  }

  return (
    <>
      <Button type="button" variant="outline" size="sm" onClick={copy}>
        {copied ? (
          <CheckIcon aria-hidden="true" />
        ) : (
          <ClipboardTextIcon aria-hidden="true" />
        )}
        {copied ? t('report.copied') : t('report.copy')}
      </Button>
      {copied ? (
        <span role="status" className="sr-only">
          {t('report.copied')}
        </span>
      ) : null}
    </>
  )
}
