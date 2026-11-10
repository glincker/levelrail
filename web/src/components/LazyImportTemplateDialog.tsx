import * as React from 'react'

// The dialog and its form code are only needed once someone opens the import.
const ImportTemplateDialog = React.lazy(() =>
  import('./ImportTemplateDialog').then((m) => ({
    default: m.ImportTemplateDialog,
  })),
)

export function LazyImportTemplateDialog() {
  return (
    <React.Suspense fallback={null}>
      <ImportTemplateDialog />
    </React.Suspense>
  )
}
