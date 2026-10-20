import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useRollbackAppImport,
  useStageAppImport,
  useVerifyAppImport,
  type AppImportItem,
  type AppImportSession,
} from '../queries/appImport'
import { isStaged, stateVariant } from './appImportFormat'

function ItemRow({
  item,
  canVerify,
  busy,
  onVerify,
}: {
  item: AppImportItem
  canVerify: boolean
  busy: boolean
  onVerify: () => void
}) {
  const { t } = useTranslation('migration')
  return (
    <li className="rounded-md border p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{item.name}</span>
        {item.target && item.target !== item.name ? (
          <span className="text-xs text-muted-foreground">
            {t('appImport.run.importedAs', { name: item.target })}
          </span>
        ) : null}
        <Badge variant={stateVariant[item.state]}>
          {t(`appImport.state.${item.state}`)}
        </Badge>
        <div className="ml-auto flex items-center gap-2">
          {item.target && isStaged(item.state) ? (
            <>
              <Link
                to="/apps/$name"
                params={{ name: item.target }}
                className="text-xs underline underline-offset-2"
              >
                {t('appImport.run.openApp')}
              </Link>
              <Link
                to="/apps/$name/logs"
                params={{ name: item.target }}
                className="text-xs underline underline-offset-2"
              >
                {t('appImport.run.openLogs')}
              </Link>
            </>
          ) : null}
          {canVerify && isStaged(item.state) && item.state !== 'routed' ? (
            <Button
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={onVerify}
            >
              {item.state === 'verify-failed'
                ? t('appImport.run.verifyAgain')
                : t('appImport.run.verifyOne')}
            </Button>
          ) : null}
        </div>
      </div>
      {item.reason ? (
        <p className="mt-1 break-words text-xs text-muted-foreground">
          {item.reason}
        </p>
      ) : null}
    </li>
  )
}

export function AppImportRun({
  session,
  verifyView,
  onNext,
  onBack,
}: {
  session: AppImportSession
  verifyView: boolean
  onNext: () => void
  onBack: () => void
}) {
  const { t } = useTranslation('migration')
  const stage = useStageAppImport()
  const verify = useVerifyAppImport()
  const rollback = useRollbackAppImport()
  const [confirmRollback, setConfirmRollback] = useState(false)
  const items = session.items.filter((i) => i.selected)
  const stagedCount = items.filter((i) => isStaged(i.state)).length
  const verified = items.filter(
    (i) => i.state === 'verified' || i.state === 'routed',
  ).length
  const busy =
    session.running ||
    stage.isPending ||
    verify.isPending ||
    rollback.isPending ||
    (session.states.building ?? 0) > 0
  const error = stage.error ?? verify.error ?? rollback.error

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">
          {verifyView
            ? t('appImport.run.verifyTitle')
            : t('appImport.run.stageTitle')}
        </h3>
        <p className="text-sm text-muted-foreground">
          {verifyView
            ? t('appImport.run.verifyDescription')
            : t('appImport.run.stageDescription')}
        </p>
      </div>
      {!session.connected && !verifyView ? (
        <Alert>
          <AlertDescription>{t('appImport.run.needsToken')}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        {verifyView ? (
          <Button
            disabled={busy || stagedCount === 0}
            onClick={() =>
              verify.mutate(
                { id: session.id },
                {
                  onError: (e) =>
                    toast.add({ title: e.message, type: 'error' }),
                },
              )
            }
          >
            {busy ? t('appImport.run.working') : t('appImport.run.verifyAll')}
          </Button>
        ) : (
          <Button
            disabled={busy || !session.connected}
            onClick={() => stage.mutate({ id: session.id })}
          >
            {busy
              ? t('appImport.run.working')
              : stagedCount > 0
                ? t('appImport.run.resume')
                : t('appImport.run.stage')}
          </Button>
        )}
        {stagedCount > 0 ? (
          confirmRollback ? (
            <>
              <span className="text-xs">
                {t('appImport.run.rollbackConfirm')}
              </span>
              <Button
                size="sm"
                variant="destructive"
                disabled={busy}
                onClick={() => {
                  rollback.mutate({ id: session.id })
                  setConfirmRollback(false)
                }}
              >
                {t('appImport.run.rollbackYes')}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setConfirmRollback(false)}
              >
                {t('appImport.run.cancel')}
              </Button>
            </>
          ) : (
            <Button
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => setConfirmRollback(true)}
            >
              {t('appImport.run.rollback')}
            </Button>
          )
        ) : null}
      </div>
      <p className="text-xs text-muted-foreground">
        {verifyView
          ? t('appImport.run.verifyNote')
          : t('appImport.run.stageNote')}
      </p>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription className="break-words whitespace-pre-wrap">
            {error.message}
          </AlertDescription>
        </Alert>
      ) : null}
      <ul className="space-y-2">
        {items.map((it) => (
          <ItemRow
            key={it.source_id}
            item={it}
            canVerify={verifyView}
            busy={busy}
            onVerify={() =>
              verify.mutate({ id: session.id, items: [it.source_id] })
            }
          />
        ))}
      </ul>
      <div className="flex items-center gap-2">
        <Button variant="outline" onClick={onBack}>
          {t('appImport.back')}
        </Button>
        <Button
          disabled={verifyView ? verified === 0 : stagedCount === 0}
          onClick={onNext}
        >
          {verifyView
            ? t('appImport.run.toVolumes')
            : t('appImport.run.toVerify')}
        </Button>
      </div>
    </div>
  )
}
