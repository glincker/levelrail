import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { SkipForwardIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useCancelSuperseded,
  useSetCancelSuperseded,
} from '../../queries/deployControl'

function AppSetting({ app }: { app: string }) {
  const setting = useCancelSuperseded(app)
  const setSetting = useSetCancelSuperseded(app)
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm">Auto-cancel superseded</span>
        <Switch
          checked={setting.data?.enabled ?? false}
          disabled={setting.isPending || setSetting.isPending}
          aria-label={`Auto-cancel superseded deploys for ${app}`}
          onCheckedChange={(next) => {
            setSetting.mutate(next, {
              onError: (error) => {
                toast.add({
                  title: 'Could not update the setting.',
                  description: error.message,
                  type: 'error',
                })
              },
            })
          }}
        />
      </div>
      {setting.isError && (
        <p className="text-xs text-tone-danger">{setting.error.message}</p>
      )}
      <Link
        to="/apps/$name/deploy-settings"
        params={{ name: app }}
        className="text-xs text-muted-foreground underline underline-offset-4 hover:text-foreground"
      >
        Open deploy settings
      </Link>
    </div>
  )
}

export interface CancelSupersededPopoverProps {
  apps: string[]
  selected: string
}

export function CancelSupersededPopover({
  apps,
  selected,
}: CancelSupersededPopoverProps) {
  const [picked, setPicked] = useState('')
  const options =
    selected && !apps.includes(selected) ? [selected, ...apps] : apps
  const app = selected || picked || options[0] || ''
  return (
    <Popover>
      <PopoverTrigger
        render={<Button type="button" size="sm" variant="outline" />}
      >
        <SkipForwardIcon aria-hidden="true" />
        Queue
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72">
        <PopoverHeader>
          <PopoverTitle>Auto-cancel superseded deploys</PopoverTitle>
          <PopoverDescription>
            A newer queued deploy of a branch replaces older queued ones. Builds
            already running are never canceled. Set per app.
          </PopoverDescription>
        </PopoverHeader>
        {app === '' ? (
          <p className="text-xs text-muted-foreground">No apps loaded yet.</p>
        ) : (
          <>
            {!selected && options.length > 1 && (
              <select
                aria-label="App"
                value={app}
                onChange={(e) => {
                  setPicked(e.target.value)
                }}
                className="h-8 rounded-md border border-border bg-background px-2 text-sm"
              >
                {options.map((o) => (
                  <option key={o} value={o}>
                    {o}
                  </option>
                ))}
              </select>
            )}
            <AppSetting key={app} app={app} />
          </>
        )}
      </PopoverContent>
    </Popover>
  )
}
