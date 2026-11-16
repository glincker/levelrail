import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { SkipForwardGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useCancelSuperseded,
  useSetCancelSuperseded,
} from '../../queries/deployControl'

function AppSetting({ app }: { app: string }) {
  const { t } = useTranslation('deploys')
  const setting = useCancelSuperseded(app)
  const setSetting = useSetCancelSuperseded(app)
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm">
          {t('supersededPopover.autoCancelLabel')}
        </span>
        <Switch
          checked={setting.data?.enabled ?? false}
          disabled={setting.isPending || setSetting.isPending}
          aria-label={t('supersededPopover.switchLabel', { app })}
          onCheckedChange={(next) => {
            setSetting.mutate(next, {
              onError: (error) => {
                toast.add({
                  title: t('supersededPopover.updateFailed'),
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
        {t('supersededPopover.openSettings')}
      </Link>
    </div>
  )
}

export interface CancelSupersededPopoverProps {
  apps: string[]
  selected: string
  // When set, renders a menu-driven dialog instead of the trigger popover.
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

export function CancelSupersededPopover({
  apps,
  selected,
  open,
  onOpenChange,
}: CancelSupersededPopoverProps) {
  const { t } = useTranslation('deploys')
  const [picked, setPicked] = useState('')
  const options =
    selected && !apps.includes(selected) ? [selected, ...apps] : apps
  const app = selected || picked || options[0] || ''
  const content = (
    <>
      {app === '' ? (
        <p className="text-xs text-muted-foreground">
          {t('supersededPopover.noApps')}
        </p>
      ) : (
        <>
          {!selected && options.length > 1 && (
            <select
              aria-label={t('supersededPopover.appLabel')}
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
    </>
  )
  if (open !== undefined) {
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('supersededPopover.title')}</DialogTitle>
            <DialogDescription>
              {t('supersededPopover.description')}
            </DialogDescription>
          </DialogHeader>
          {content}
        </DialogContent>
      </Dialog>
    )
  }
  return (
    <Popover>
      <PopoverTrigger
        render={<Button type="button" size="sm" variant="outline" />}
      >
        <SkipForwardGlassIcon size="1.4em" aria-hidden="true" />
        {t('supersededPopover.queue')}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72">
        <PopoverHeader>
          <PopoverTitle>{t('supersededPopover.title')}</PopoverTitle>
          <PopoverDescription>
            {t('supersededPopover.description')}
          </PopoverDescription>
        </PopoverHeader>
        {content}
      </PopoverContent>
    </Popover>
  )
}
