import type { FormEvent, ReactNode } from 'react'
import { DraftRestoredNotice } from './DraftRestoredNotice'

// CreateFormShell is the <form> wrapper plus the optional "restored from
// draft" notice every create-resource step-2 form (CreateAppFields,
// CreateDatabaseFields) repeats identically before its own fields
// diverge.
export function CreateFormShell({
  onSubmit,
  restoredFromDraft,
  onDiscardDraft,
  onDismissDraftNotice,
  children,
}: {
  onSubmit: (e: FormEvent<HTMLFormElement>) => unknown
  restoredFromDraft: boolean
  onDiscardDraft: () => void
  onDismissDraftNotice: () => void
  children: ReactNode
}) {
  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="space-y-4"
    >
      {restoredFromDraft ? (
        <DraftRestoredNotice
          onDiscard={onDiscardDraft}
          onDismiss={onDismissDraftNotice}
        />
      ) : null}

      {children}
    </form>
  )
}
