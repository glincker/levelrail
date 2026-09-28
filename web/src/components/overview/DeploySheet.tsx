import { DeployTriggerForm } from '../DeployTriggerForm'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'

export type DeployTab = 'existing-image' | 'build-from-source'

export function DeploySheet({
  appName,
  tab,
  onClose,
}: {
  appName: string
  tab: DeployTab | null
  onClose: () => void
}) {
  return (
    <Sheet
      open={tab !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <SheetContent
        side="right"
        className="overflow-y-auto data-[side=right]:w-full data-[side=right]:sm:max-w-lg"
      >
        <SheetHeader>
          <SheetTitle>Deploy</SheetTitle>
          <SheetDescription>
            Ship an image or build from source.
          </SheetDescription>
        </SheetHeader>
        <div className="p-4 pt-0">
          {tab ? (
            <DeployTriggerForm key={tab} appName={appName} defaultTab={tab} />
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
