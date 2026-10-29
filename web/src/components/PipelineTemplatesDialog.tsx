import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LayoutGridGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useSavePipeline } from '../queries/pipelines'
import {
  usePipelineTemplate,
  usePipelineTemplates,
  type PipelineTemplate,
} from '../queries/pipelineUx'

export function PipelineTemplatesDialog({ appName }: { appName: string }) {
  const { t } = useTranslation('pipelines')
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [pickedId, setPickedId] = useState('')
  const [name, setName] = useState('')
  const { data, isLoading, error } = usePipelineTemplates(open)
  const fetchTemplate = usePipelineTemplate()
  const save = useSavePipeline(appName)
  const busy = fetchTemplate.isPending || save.isPending

  function pick(tpl: PipelineTemplate) {
    setPickedId(tpl.id)
    setName(tpl.id)
  }

  function create() {
    fetchTemplate.mutate(pickedId, {
      onSuccess: (full) =>
        save.mutate(
          { req: { name, yaml: full.yaml ?? '' } },
          {
            onSuccess: (p) => {
              setOpen(false)
              toast.add({
                title: t('templates.created', { name: p.name }),
                type: 'success',
              })
              void navigate({
                to: '/apps/$name/pipelines/$pipeline',
                params: { name: appName, pipeline: p.name },
              })
            },
            onError: (e) => toast.add({ title: e.message, type: 'error' }),
          },
        ),
      onError: (e) => toast.add({ title: e.message, type: 'error' }),
    })
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <LayoutGridGlassIcon size="1.2em" aria-hidden="true" />
        {t('templates.open')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('templates.title')}</DialogTitle>
          <DialogDescription>{t('templates.description')}</DialogDescription>
        </DialogHeader>
        {isLoading ? (
          <Skeleton className="h-40 w-full" />
        ) : error ? (
          <p className="text-sm text-destructive">
            {t('templates.loadFailed')}
          </p>
        ) : (
          <ul className="grid max-h-80 gap-2 overflow-y-auto sm:grid-cols-2">
            {(data ?? []).map((tpl) => (
              <li key={tpl.id}>
                <button
                  type="button"
                  aria-pressed={pickedId === tpl.id}
                  onClick={() => pick(tpl)}
                  className="h-full w-full rounded-lg border border-border p-3 text-left text-sm transition-colors hover:bg-muted aria-pressed:border-primary aria-pressed:bg-muted"
                >
                  <span className="flex items-center justify-between gap-2">
                    <span className="font-medium text-foreground">
                      {tpl.name}
                    </span>
                    <Badge variant="outline">{tpl.language}</Badge>
                  </span>
                  <span className="mt-1 block text-xs text-muted-foreground">
                    {tpl.description}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            aria-label={t('templates.name')}
            placeholder={t('templates.name')}
            className="w-56"
            disabled={pickedId === ''}
          />
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t('templates.cancel')}
          </Button>
          <Button
            disabled={pickedId === '' || name === '' || busy}
            onClick={create}
          >
            {busy ? t('templates.creating') : t('templates.use')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
