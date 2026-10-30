import { SuggestionList } from '@/components/kit'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'
import { DiagnosisFixes } from '../DiagnosisFixes'
import type { SuggestionDescriptor } from './suggestions'
import { toSuggestionItems, useSuggestionActions } from './useSuggestionActions'

export function OverviewSuggestions({
  app,
  descriptors,
  dismiss,
  latest,
  anchorId,
}: {
  app: AppDetail
  descriptors: SuggestionDescriptor[]
  dismiss: (id: string) => void
  latest?: DeployAttempt
  anchorId?: string
}) {
  const actions = useSuggestionActions(app, latest)
  if (descriptors.length === 0) return null
  return (
    <div id={anchorId}>
      {actions.confirmDialog}
      <SuggestionList
        items={toSuggestionItems(descriptors, actions, dismiss)}
      />
      <Dialog open={actions.fixOpen} onOpenChange={actions.setFixOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Suggested fixes</DialogTitle>
          </DialogHeader>
          <DiagnosisFixes appName={app.name} deployId={latest?.id} />
        </DialogContent>
      </Dialog>
    </div>
  )
}
