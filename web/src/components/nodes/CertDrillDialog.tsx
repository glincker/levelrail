import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleMinusGlassIcon,
  CircleXGlassIcon,
  ShieldCheckGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useNodeCertDrill, type CertDrillStep } from '../../queries/nodeTrust'
import type { NodeResource } from '../../types/nodeDetail'

function StepIcon({ status }: { status: CertDrillStep['status'] }) {
  if (status === 'pass') {
    return (
      <CircleCheckGlassIcon
        className="size-5 shrink-0 text-green-600"
        aria-hidden="true"
      />
    )
  }
  if (status === 'fail') {
    return (
      <CircleXGlassIcon
        className="size-5 shrink-0 text-destructive"
        aria-hidden="true"
      />
    )
  }
  return (
    <CircleMinusGlassIcon
      className="size-5 shrink-0 text-muted-foreground"
      aria-hidden="true"
    />
  )
}

export function CertDrillDialog({ node }: { node: NodeResource }) {
  const { t } = useTranslation('nodes')
  const [open, setOpen] = useState(false)
  const drill = useNodeCertDrill()
  const result = drill.data
  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={node.cert?.state === 'revoked' || node.status !== 'online'}
        title={node.status !== 'online' ? t('trust.renewOffline') : undefined}
        onClick={() => {
          setOpen(true)
        }}
      >
        <ShieldCheckGlassIcon
          className="size-5 text-tone-success"
          aria-hidden="true"
        />
        {t('trust.drill.button')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (!next) drill.reset()
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {t('trust.drill.title', { name: node.name })}
            </DialogTitle>
            <DialogDescription>
              {t('trust.drill.description')}
            </DialogDescription>
          </DialogHeader>
          {drill.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{drill.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {result ? (
            <div className="space-y-3" role="status">
              <ul className="space-y-2 text-sm">
                {result.steps.map((s) => (
                  <li key={s.id} className="flex items-start gap-2">
                    <StepIcon status={s.status} />
                    <span>
                      <span className="font-medium">
                        {t(`trust.drill.steps.${s.id}`)}
                      </span>
                      <span className="block text-muted-foreground">
                        {s.detail}
                      </span>
                    </span>
                  </li>
                ))}
              </ul>
              <p className="text-sm font-medium">
                {result.ok
                  ? t('trust.drill.passed', {
                      from: result.before.generation,
                      to: result.after.generation,
                    })
                  : t('trust.drill.failed')}
              </p>
            </div>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setOpen(false)
                drill.reset()
              }}
            >
              {t('trust.drill.close')}
            </Button>
            <Button
              type="button"
              disabled={drill.isPending}
              onClick={() => {
                drill.mutate(node.id)
              }}
            >
              {drill.isPending
                ? t('trust.drill.running')
                : result
                  ? t('trust.drill.again')
                  : t('trust.drill.run')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
