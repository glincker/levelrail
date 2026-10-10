import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  CaretRightIcon,
  GlobeHemisphereWestIcon,
  ShieldCheckIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import type {
  ExposureClass,
  ExposureFinding,
  ExposureSeverity,
} from '../../types/exposure'
import { useExposure, useRemoveRestriction } from '../../queries/exposure'
import { ExposureRestrictDialog } from './ExposureRestrictDialog'

const SEVERITY_VARIANT: Record<
  ExposureSeverity,
  'destructive' | 'warning' | 'muted' | 'default'
> = {
  high: 'destructive',
  medium: 'warning',
  low: 'default',
  info: 'muted',
}

const CLASS_VARIANT: Record<
  ExposureClass,
  'destructive' | 'warning' | 'success' | 'muted'
> = {
  exposed: 'destructive',
  unknown: 'warning',
  restricted: 'success',
  private: 'muted',
  loopback: 'muted',
}

function FindingRow({
  finding,
  local,
  open,
  onToggle,
}: {
  finding: ExposureFinding
  local: boolean
  open: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation('exposure')
  const [restricting, setRestricting] = useState(false)
  const remove = useRemoveRestriction()
  const port = `${finding.host_port}/${finding.protocol}`

  function onRemove() {
    remove.mutate(
      { port: finding.host_port, protocol: finding.protocol },
      {
        onSuccess: () =>
          toast.add({ title: t('removedToast'), type: 'success' }),
        onError: (err) =>
          toast.add({
            title: t('failedToast'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Fragment>
      <TableRow>
        <TableCell>
          <Badge variant={SEVERITY_VARIANT[finding.severity]}>
            {t(`severity.${finding.severity}`)}
          </Badge>
        </TableCell>
        <TableCell className="font-mono text-xs">{port}</TableCell>
        <TableCell className="text-xs">{finding.container}</TableCell>
        <TableCell className="text-xs">
          {t(`owner.${finding.owner.kind}`, { name: finding.owner.name })}
        </TableCell>
        <TableCell className="font-mono text-xs">
          {finding.binds.join(', ')}
        </TableCell>
        <TableCell>
          <Badge variant={CLASS_VARIANT[finding.class]}>
            {t(`class.${finding.class}`)}
          </Badge>
        </TableCell>
        <TableCell className="text-right">
          <Button
            variant="ghost"
            size="sm"
            aria-expanded={open}
            onClick={onToggle}
          >
            {open ? (
              <CaretDownIcon aria-hidden="true" />
            ) : (
              <CaretRightIcon aria-hidden="true" />
            )}
            {open ? t('hideDetails') : t('details')}
          </Button>
        </TableCell>
      </TableRow>
      {open ? (
        <TableRow>
          <TableCell colSpan={7} className="space-y-2 bg-muted/30 text-sm">
            <p>{finding.explanation}</p>
            {finding.recommendation ? (
              <p className="text-muted-foreground">{finding.recommendation}</p>
            ) : null}
            {finding.restriction ? (
              <p className="text-xs">
                {t('restrictedTo', {
                  sources: finding.restriction.allow.join(', '),
                })}
              </p>
            ) : null}
            {finding.outside_check.status !== 'not_run' ? (
              <p className="text-xs text-muted-foreground">
                {t(`outside.${finding.outside_check.status}`)}
                {finding.outside_check.detail
                  ? `: ${finding.outside_check.detail}`
                  : ''}
              </p>
            ) : null}
            {finding.cannot_restrict_reason ? (
              <p className="text-xs text-muted-foreground">
                {finding.cannot_restrict_reason}
              </p>
            ) : null}
            <div className="flex gap-2">
              {local && finding.can_restrict ? (
                <Button size="sm" onClick={() => setRestricting(true)}>
                  {t('restrict')}
                </Button>
              ) : null}
              {local && finding.restriction ? (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={remove.isPending}
                  onClick={onRemove}
                >
                  {t('remove')}
                </Button>
              ) : null}
            </div>
            {restricting ? (
              <ExposureRestrictDialog
                finding={finding}
                open={restricting}
                onOpenChange={setRestricting}
              />
            ) : null}
          </TableCell>
        </TableRow>
      ) : null}
    </Fragment>
  )
}

/** ExposureCard lists every published container port and how reachable it is; it is read-only until the operator opens the restrict dialog. */
export function ExposureCard() {
  const { t } = useTranslation('exposure')
  const [probe, setProbe] = useState(false)
  const [openKey, setOpenKey] = useState<string | null>(null)
  const { data, isFetching } = useExposure(probe)
  if (!data) return null

  const rows = data.nodes.flatMap((n) =>
    n.findings.map((f) => ({ node: n, finding: f })),
  )
  const unreadable = data.nodes.filter((n) => !n.rules_readable && n.local)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          <GlobeHemisphereWestIcon
            className="size-4 text-muted-foreground"
            aria-hidden="true"
          />
          {t('title')}
          <Button
            variant="outline"
            size="sm"
            className="ml-auto"
            disabled={isFetching}
            onClick={() => setProbe(true)}
          >
            {isFetching && probe ? t('probing') : t('probe')}
          </Button>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <p className="text-muted-foreground">{t('description')}</p>
        {probe ? (
          <p className="text-xs text-muted-foreground">{t('probeNote')}</p>
        ) : null}
        {data.nodes
          .filter((n) => n.status !== 'ok')
          .map((n) => (
            <p key={n.node_id || 'local'} className="text-xs text-destructive">
              {n.node_name}: {t('nodeUnreachable', { error: n.error })}
            </p>
          ))}
        {unreadable.map((n) => (
          <p
            key={n.node_id || 'local'}
            className="text-xs text-muted-foreground"
          >
            {t('rulesUnreadable', { reason: n.rules_note })}
          </p>
        ))}
        {rows.length === 0 ? (
          <EmptyState
            icon={<ShieldCheckIcon className="size-5" />}
            title={t('emptyTitle')}
            description={t('emptyBody')}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('columns.severity')}</TableHead>
                <TableHead>{t('columns.port')}</TableHead>
                <TableHead>{t('columns.container')}</TableHead>
                <TableHead>{t('columns.owner')}</TableHead>
                <TableHead>{t('columns.bind')}</TableHead>
                <TableHead>{t('columns.state')}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map(({ node, finding }) => {
                const key = `${node.node_id}:${finding.protocol}:${finding.host_port}:${finding.container}`
                return (
                  <FindingRow
                    key={key}
                    finding={finding}
                    local={node.local}
                    open={openKey === key}
                    onToggle={() => setOpenKey(openKey === key ? null : key)}
                  />
                )
              })}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
