import * as React from 'react'
import type { ComponentProps } from 'react'

// The dialog and its issue builder are only needed after someone opens it.
const ReportIssueDialog = React.lazy(() =>
  import('./ReportIssueDialog').then((m) => ({ default: m.ReportIssueDialog })),
)

type Props = ComponentProps<typeof ReportIssueDialog>

export function LazyReportIssueDialog(props: Props) {
  const [wasOpen, setWasOpen] = React.useState(props.open)
  if (props.open && !wasOpen) setWasOpen(true)
  if (!wasOpen) return null
  return (
    <React.Suspense fallback={null}>
      <ReportIssueDialog {...props} />
    </React.Suspense>
  )
}
