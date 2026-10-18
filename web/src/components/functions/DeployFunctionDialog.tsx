import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { RocketLaunchIcon } from '@phosphor-icons/react/dist/ssr'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { useDeployFunction } from '../../queries/functions'

const DEFAULT_PORT = 8080
const DEFAULT_IDLE_MINUTES = 10
const DEFAULT_REF = 'main'

function Row({
  id,
  label,
  hint,
  children,
}: Readonly<{
  id: string
  label: string
  hint?: string
  children: React.ReactNode
}>) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  )
}

export function DeployFunctionDialog() {
  const { t } = useTranslation('functions')
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [repoUrl, setRepoUrl] = useState('')
  const [ref, setRef] = useState(DEFAULT_REF)
  const [directory, setDirectory] = useState('')
  const [port, setPort] = useState(String(DEFAULT_PORT))
  const [idle, setIdle] = useState(String(DEFAULT_IDLE_MINUTES))
  const deploy = useDeployFunction()

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    deploy.mutate(
      {
        name: name.trim(),
        repoUrl: repoUrl.trim(),
        ref: ref.trim() || DEFAULT_REF,
        directory: directory.trim(),
        port: Number(port) || DEFAULT_PORT,
        idleMinutes: Number(idle) || DEFAULT_IDLE_MINUTES,
      },
      {
        onSuccess: () => {
          toast.add({
            title: t('deploy.started', { name: name.trim() }),
            type: 'success',
          })
          setOpen(false)
          setName('')
        },
        onError: (err) => {
          toast.add({
            title: t('deploy.failed', {
              name: name.trim(),
              message: err.message,
            }),
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button type="button" />}>
        <RocketLaunchIcon aria-hidden="true" />
        {t('deploy.open')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={onSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t('deploy.title')}</DialogTitle>
            <DialogDescription>{t('deploy.description')}</DialogDescription>
          </DialogHeader>
          <Row id="fn-name" label={t('deploy.name')}>
            <Input
              id="fn-name"
              value={name}
              required
              pattern="[a-z0-9]([a-z0-9-]*[a-z0-9])?"
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
          </Row>
          <Row id="fn-repo" label={t('deploy.repo')}>
            <Input
              id="fn-repo"
              type="url"
              value={repoUrl}
              required
              onChange={(e) => {
                setRepoUrl(e.target.value)
              }}
            />
          </Row>
          <div className="grid grid-cols-2 gap-3">
            <Row id="fn-ref" label={t('deploy.ref')}>
              <Input
                id="fn-ref"
                value={ref}
                onChange={(e) => {
                  setRef(e.target.value)
                }}
              />
            </Row>
            <Row id="fn-dir" label={t('deploy.directory')}>
              <Input
                id="fn-dir"
                value={directory}
                onChange={(e) => {
                  setDirectory(e.target.value)
                }}
              />
            </Row>
          </div>
          <p className="-mt-2 text-xs text-muted-foreground">
            {t('deploy.directoryHint')}
          </p>
          <div className="grid grid-cols-2 gap-3">
            <Row id="fn-port" label={t('deploy.port')}>
              <Input
                id="fn-port"
                type="number"
                min={1}
                max={65535}
                value={port}
                onChange={(e) => {
                  setPort(e.target.value)
                }}
              />
            </Row>
            <Row id="fn-idle" label={t('deploy.idle')}>
              <Input
                id="fn-idle"
                type="number"
                min={5}
                value={idle}
                onChange={(e) => {
                  setIdle(e.target.value)
                }}
              />
            </Row>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setOpen(false)
              }}
            >
              {t('deploy.cancel')}
            </Button>
            <Button type="submit" disabled={deploy.isPending}>
              {t('deploy.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
