import { useState } from 'react'
import { PlusCircleIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateDeployNotifyTarget } from '../queries/deployNotify'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'
import { CHANNEL_KIND_LABEL } from './notificationChannelKind'

// Attach-an-already-connected-channel flow, replacing the old "type a
// URL, pick a kind" form. Mirrors StorageAttachmentCard's empty state.
export function CreateDeployNotifyTargetDialog({
  appName,
}: {
  appName: string
}) {
  const [open, setOpen] = useState(false)
  const [channelId, setChannelId] = useState('')
  const [enabled, setEnabled] = useState(true)
  const channelsQuery = useNotificationChannelsOptional()
  const channels = channelsQuery.data ?? []
  const createTarget = useCreateDeployNotifyTarget(appName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setChannelId('')
      setEnabled(true)
      createTarget.reset()
    }
  }

  function handleSubmit() {
    if (!channelId) {
      return
    }
    createTarget.mutate(
      { channel_id: channelId, enabled },
      {
        onSuccess: () => {
          handleOpenChange(false)
          toast.add({ title: 'Deploy notify target added.', type: 'success' })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button size="sm" />}>Add target</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <PlusCircleIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Add deploy notify target
          </DialogTitle>
          <DialogDescription>
            Notified once per deploy attempt reaching a terminal state
            (succeeded or failed), separate from this app&apos;s threshold and
            crashloop alert rules above.
          </DialogDescription>
        </DialogHeader>

        {channels.length === 0 ? (
          <>
            <p className="text-sm text-muted-foreground">
              No channels connected yet. Connect one from Settings &rarr;
              Notification channels first.
            </p>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                Close
              </Button>
            </DialogFooter>
          </>
        ) : (
          <div className="space-y-4">
            <Field>
              <FieldLabel htmlFor="deploy-notify-channel">Channel</FieldLabel>
              <Select
                value={channelId}
                onValueChange={(value) => {
                  if (value) {
                    setChannelId(value)
                  }
                }}
              >
                <SelectTrigger id="deploy-notify-channel" className="w-full">
                  <SelectValue placeholder="Choose a connected channel" />
                </SelectTrigger>
                <SelectContent>
                  {channels.map((channel) => (
                    <SelectItem key={channel.id} value={channel.id}>
                      {channel.name} ({CHANNEL_KIND_LABEL[channel.kind]})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldDescription>
                Reuses a channel connection from Settings &rarr; Notification
                channels; the same connection can back other apps too.
              </FieldDescription>
            </Field>

            <Field orientation="horizontal">
              <Switch
                id="deploy-notify-enabled"
                checked={enabled}
                onCheckedChange={setEnabled}
              />
              <FieldLabel htmlFor="deploy-notify-enabled">Enabled</FieldLabel>
            </Field>

            {createTarget.isError ? (
              <p className="text-sm text-destructive">
                {createTarget.error.message}
              </p>
            ) : null}

            <DialogFooter>
              <Button
                type="button"
                disabled={createTarget.isPending || !channelId}
                onClick={handleSubmit}
              >
                {createTarget.isPending ? 'Adding...' : 'Add target'}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
