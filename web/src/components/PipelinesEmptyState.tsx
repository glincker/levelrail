import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { PlusIcon, TreeStructureIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { appListQueryOptions } from '../queries/apps'

export const MINIMAL_PIPELINE_YAML = `version: 1
name: ci
on:
  push:
    branches: [main]
jobs:
  test:
    image: golang:1.23
    steps:
      - run: go test ./...
`

function AppPickerDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const navigate = useNavigate()
  const { data: apps, isLoading } = useQuery({
    ...appListQueryOptions(),
    enabled: open,
  })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Choose an app</DialogTitle>
          <DialogDescription>
            A pipeline belongs to one app. Pick the app to open its Pipelines
            tab.
          </DialogDescription>
        </DialogHeader>
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading apps...</p>
        ) : (apps ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">
            There are no apps yet.{' '}
            <Link to="/apps" className="underline underline-offset-2">
              Create an app
            </Link>{' '}
            first, then add a pipeline to it.
          </p>
        ) : (
          <ul className="max-h-72 space-y-1 overflow-y-auto">
            {(apps ?? []).map((app) => (
              <li key={app.name}>
                <Button
                  variant="ghost"
                  className="w-full justify-start"
                  onClick={() => {
                    onOpenChange(false)
                    void navigate({
                      to: '/apps/$name/pipelines',
                      params: { name: app.name },
                    })
                  }}
                >
                  {app.name}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </DialogContent>
    </Dialog>
  )
}

export function PipelinesEmptyState() {
  const [picking, setPicking] = useState(false)
  return (
    <>
      <EmptyState
        icon={<TreeStructureIcon className="size-5" />}
        title="No pipeline runs yet"
        description="A pipeline is a YAML file that runs tests, builds an image, waits for an approval, and deploys when code changes. Each pipeline belongs to one app and runs on your own nodes."
        action={
          <Button onClick={() => setPicking(true)}>
            <PlusIcon aria-hidden="true" />
            Create a pipeline
          </Button>
        }
        helpPath="/pipelines"
        helpLabel="Pipelines docs"
      >
        <pre
          aria-label="Example pipeline"
          className="w-full max-w-md overflow-x-auto rounded-md border border-border bg-muted p-3 text-left font-mono text-xs text-foreground"
        >
          <code>{MINIMAL_PIPELINE_YAML}</code>
        </pre>
      </EmptyState>
      <AppPickerDialog open={picking} onOpenChange={setPicking} />
    </>
  )
}
