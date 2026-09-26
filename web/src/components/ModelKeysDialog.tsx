import { GaugeIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { ModelKeysPanel } from './ModelKeysPanel'
import { ModelUsageCard } from './ModelUsageCard'

// Keys and usage for one model, opened from its row.
export function ModelKeysDialog({
  name,
  baseUrl,
  disabled,
}: {
  name: string
  baseUrl?: string
  disabled?: boolean
}) {
  return (
    <Dialog>
      <DialogTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={`Keys and usage of ${name}`}
            title="Keys and usage"
            disabled={disabled}
          />
        }
      >
        <GaugeIcon aria-hidden="true" />
      </DialogTrigger>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>&ldquo;{name}&rdquo; keys and usage</DialogTitle>
          <DialogDescription>
            Named keys with their own limits, and what each one used.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-6">
          <ModelKeysPanel modelName={name} baseUrl={baseUrl} />
          <ModelUsageCard modelName={name} />
        </div>
      </DialogContent>
    </Dialog>
  )
}
