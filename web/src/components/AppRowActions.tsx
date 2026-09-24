import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  ArrowClockwiseIcon,
  ArrowSquareOutIcon,
  ClockCounterClockwiseIcon,
  DotsThreeIcon,
  PauseIcon,
  PlayIcon,
  RocketLaunchIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLinkItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { toast } from '@/components/ui/toast'
import { useRedeployApp } from '../hooks/useRedeployApp'
import { useRestartApp, useStartApp, useStopApp } from '../queries/apps'
import type { AppListEntry } from '../types/appDetail'

export function AppRowActions({ app }: { app: AppListEntry }) {
  const [confirmStop, setConfirmStop] = useState(false)
  const restartApp = useRestartApp()
  const stopApp = useStopApp()
  const startApp = useStartApp()
  const { redeploy } = useRedeployApp(app.name, app.image)
  const domain = app.domains?.[0]

  function run(
    mutate: typeof restartApp.mutate,
    message: string,
    after?: () => void,
  ) {
    mutate(app.name, {
      onSuccess: () => {
        toast.add({ title: message, type: 'success' })
        after?.()
      },
      onError: (error) => {
        toast.add({ title: error.message, type: 'error' })
        after?.()
      },
    })
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              className="relative z-10 justify-self-end"
              aria-label={`Actions for ${app.name}`}
            />
          }
        >
          <DotsThreeIcon weight="bold" />
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem
            onClick={() => run(restartApp.mutate, `Restarting "${app.name}".`)}
          >
            <ArrowClockwiseIcon />
            Restart
          </DropdownMenuItem>
          {app.suspended ? (
            <DropdownMenuItem
              onClick={() => run(startApp.mutate, `Starting "${app.name}".`)}
            >
              <PlayIcon />
              Start
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem onClick={() => setConfirmStop(true)}>
              <PauseIcon />
              Stop
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onClick={redeploy}>
            <RocketLaunchIcon />
            Redeploy
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            render={<Link to="/apps/$name/logs" params={{ name: app.name }} />}
          >
            <TerminalWindowIcon />
            View logs
          </DropdownMenuItem>
          <DropdownMenuItem
            render={
              <Link to="/apps/$name/deploys" params={{ name: app.name }} />
            }
          >
            <ClockCounterClockwiseIcon />
            View deploys
          </DropdownMenuItem>
          {domain ? (
            <DropdownMenuLinkItem
              href={`https://${domain}`}
              target="_blank"
              rel="noreferrer"
            >
              <ArrowSquareOutIcon />
              Open domain
            </DropdownMenuLinkItem>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <Dialog open={confirmStop} onOpenChange={setConfirmStop}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Stop &ldquo;{app.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              Running containers are removed until you start the app again. Its
              config, env, and domains are kept.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmStop(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={stopApp.isPending}
              onClick={() =>
                run(stopApp.mutate, `Stopping "${app.name}".`, () =>
                  setConfirmStop(false),
                )
              }
            >
              Stop app
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
