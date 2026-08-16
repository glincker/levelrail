import { useState } from 'react'
import {
  CloudArrowUpIcon,
  PlugsConnectedIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useSetAppStorage, useClearAppStorage } from '../queries/apps'
import { useBackupTargetsOptional } from '../queries/backupTargets'
import { useStorageEnvKeysOptional } from '../queries/storageEnvKeys'
import type { AppDetail } from '../types/appDetail'

// Attach/detach card for PUT/DELETE /api/v1/apps/{name}/storage
// (internal/api/apps_storage.go): lets an app claim an already-connected
// backup target (settings/backup-targets.tsx's own connect flow) as its
// object-storage credential source, the "lightweight self-hosted AWS"
// story from the repo plan: attach a bucket the same way an operator
// attaches a database, credentials just show up as env vars
// (S3_ENDPOINT/S3_BUCKET/S3_REGION/S3_ACCESS_KEY_ID/S3_SECRET_ACCESS_KEY,
// internal/reconcile/application's resolveStorageEnv doc comment), no
// manual AWS SDK config by the app developer.
//
// Deliberately reuses backup_targets rather than a second "storage
// target" concept: the same bucket connection an operator may already
// use for a database's scheduled backups can back an app's object
// storage too, and one target can back more than one app at once (no
// uniqueness constraint, migrations/0030_service_storage_target.sql's
// own comment), so nothing here needs to know or care whether a target
// is "already in use" elsewhere.
export function StorageAttachmentCard({ app }: { app: AppDetail }) {
  const [selectedTargetId, setSelectedTargetId] = useState('')
  const targetsQuery = useBackupTargetsOptional()
  const targets = targetsQuery.data ?? []
  const setAppStorage = useSetAppStorage()
  const clearAppStorage = useClearAppStorage()
  const pending = setAppStorage.isPending || clearAppStorage.isPending

  const attachedTarget = targets.find((t) => t.id === app.storage_target_id)
  const storageEnvKeysQuery = useStorageEnvKeysOptional()

  // Only checks literal env.* keys: secret-backed env var NAMES aren't
  // exposed on this app resource today (SecretEnv is write-only from
  // the frontend's point of view, see SecretsEditor.tsx), so a
  // colliding secret key still silently loses to the attached storage
  // target's value with no pre-attach warning, the narrower remaining
  // half of this gap. This still catches the more common case (a plain
  // env var the operator can see and typed themselves) up front rather
  // than leaving the whole thing silent.
  const collidingEnvKeys = (storageEnvKeysQuery.data ?? []).filter(
    (key) => app.env?.[key] !== undefined,
  )

  function handleAttach() {
    if (!selectedTargetId) {
      return
    }
    setAppStorage.mutate(
      { name: app.name, storageTargetId: selectedTargetId },
      {
        onSuccess: () => {
          setSelectedTargetId('')
          toast.add({
            title: 'Storage attached.',
            description: `${app.name} now gets its bucket credentials as env vars.`,
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: 'Could not attach storage.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  function handleDetach() {
    clearAppStorage.mutate(app.name, {
      onError: (error) => {
        toast.add({
          title: 'Could not detach storage.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Object storage</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {app.storage_target_id ? (
          <div className="flex items-center justify-between gap-4">
            <div className="flex items-start gap-2">
              <PlugsConnectedIcon
                className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
              <div>
                <p className="text-sm font-medium text-foreground">
                  {attachedTarget?.name ?? app.storage_target_id}
                </p>
                <p className="text-sm text-muted-foreground">
                  {attachedTarget
                    ? `Bucket "${attachedTarget.bucket}". Credentials are injected as S3_* env vars at container start.`
                    : 'Credentials are injected as S3_* env vars at container start.'}
                </p>
              </div>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={pending}
              onClick={handleDetach}
            >
              {clearAppStorage.isPending ? 'Detaching...' : 'Detach'}
            </Button>
          </div>
        ) : (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              No storage attached. Attach a connected bucket to inject its
              credentials as env vars, no manual AWS SDK config needed.
            </p>
            {targets.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No buckets connected yet. Connect one from Settings &rarr;
                Backup targets first.
              </p>
            ) : (
              <Field>
                <FieldLabel htmlFor="storage-attach-target">
                  Bucket
                </FieldLabel>
                {collidingEnvKeys.length > 0 ? (
                  <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
                    <WarningIcon className="mt-0.5 size-4 shrink-0" />
                    <p className="text-sm">
                      This app already has{' '}
                      {collidingEnvKeys.length === 1
                        ? 'an env var'
                        : 'env vars'}{' '}
                      named{' '}
                      <code className="font-mono">
                        {collidingEnvKeys.join(', ')}
                      </code>
                      . Attaching storage will silently override{' '}
                      {collidingEnvKeys.length === 1 ? 'it' : 'them'} at
                      container start.
                    </p>
                  </div>
                ) : null}
                <div className="flex items-center gap-2">
                  <Select
                    value={selectedTargetId}
                    onValueChange={(value) => {
                      if (value) {
                        setSelectedTargetId(value)
                      }
                    }}
                  >
                    <SelectTrigger
                      id="storage-attach-target"
                      className="w-full"
                    >
                      <SelectValue placeholder="Choose a connected bucket" />
                    </SelectTrigger>
                    <SelectContent>
                      {targets.map((t) => (
                        <SelectItem key={t.id} value={t.id}>
                          {t.name} ({t.bucket})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    type="button"
                    size="sm"
                    disabled={pending || !selectedTargetId}
                    onClick={handleAttach}
                  >
                    <CloudArrowUpIcon className="size-3.5" aria-hidden="true" />
                    {setAppStorage.isPending ? 'Attaching...' : 'Attach'}
                  </Button>
                </div>
                <FieldDescription>
                  Reuses a bucket connection from Settings &rarr; Backup
                  targets; the same connection can back a database&apos;s
                  scheduled backups too.
                </FieldDescription>
              </Field>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
