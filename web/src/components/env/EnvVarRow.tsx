import { useTranslation } from 'react-i18next'
import {
  EllipsisGlassIcon,
  LockGlassIcon,
  PlusGlassIcon,
  ReplyGlassIcon,
  ShieldCheckGlassIcon,
  ShieldOffGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { MASKED_VALUE, type EnvRow } from '../../lib/envRows'
import type { EnvTier } from '../../lib/envProvenance'
import { EnvVarValue } from './EnvVarValue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export const ENV_ROW_GRID =
  'grid gap-2 px-3 py-3 sm:grid-cols-[minmax(0,13rem)_minmax(0,1fr)_5.5rem_9rem_2.5rem] sm:items-center'

export interface EnvVarRowProps {
  row: EnvRow
  shadows?: EnvTier
  revealed?: string
  expiresAt?: number
  canReveal: boolean
  canConvert: boolean
  onChange: (row: EnvRow) => void
  onReveal: () => void
  onHide: () => void
  onCopy: (value: string) => void
  onOverride: () => void
  onSensitivity: (value: boolean | null) => void
  onConvert: () => void
}

export function EnvVarRow(props: EnvVarRowProps) {
  const { row, shadows } = props
  const { t } = useTranslation('envEditor')
  const tierLabel = (tier: EnvTier) => t(`scope.tiers.${tier}`)
  const inherited = row.origin === 'inherited'
  const existingPlain =
    row.origin === 'own' && row.type === 'plain' && row.original !== undefined

  return (
    <div
      role="listitem"
      className={`${ENV_ROW_GRID} ${row.removed ? 'opacity-60' : ''}`}
    >
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <span
          className={`font-mono text-sm break-all ${row.removed ? 'line-through' : ''}`}
        >
          {row.key}
        </span>
        {row.sensitive && row.type === 'plain' ? (
          <Badge variant="outline" className="gap-1">
            <LockGlassIcon size="1.4em" aria-hidden />
            {t('sensitive.badge')}
          </Badge>
        ) : null}
      </div>

      <div className="min-w-0">
        {inherited ? (
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate font-mono text-sm text-muted-foreground">
              {row.value === MASKED_VALUE ? MASKED_VALUE : row.value}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={props.onOverride}
            >
              <PlusGlassIcon size="1.4em" />
              {t('actions.override')}
            </Button>
          </div>
        ) : row.removed ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => {
              props.onChange({ ...row, removed: false })
            }}
          >
            <ReplyGlassIcon size="1.4em" />
            {t('actions.undoRemove')}
          </Button>
        ) : (
          <EnvVarValue
            row={row}
            revealed={props.revealed}
            expiresAt={props.expiresAt}
            canReveal={props.canReveal}
            onChange={props.onChange}
            onReveal={props.onReveal}
            onHide={props.onHide}
            onCopy={props.onCopy}
          />
        )}
      </div>

      <div>
        <Badge variant={row.type === 'plain' ? 'muted' : 'accent'}>
          {t(`types.${row.type}`)}
        </Badge>
      </div>

      <div className="text-xs text-muted-foreground">
        {inherited && row.tier
          ? t('scope.inherited', { tier: tierLabel(row.tier) })
          : shadows
            ? t('scope.overrides', { tier: tierLabel(shadows) })
            : t('scope.own')}
      </div>

      <div className="flex justify-end">
        {inherited || row.removed ? null : (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('actions.rowMenu', { key: row.key })}
                />
              }
            >
              <EllipsisGlassIcon size="1.4em" />
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              {existingPlain ? (
                <>
                  {row.sensitive ? (
                    <DropdownMenuItem
                      onClick={() => {
                        props.onSensitivity(false)
                      }}
                    >
                      <ShieldOffGlassIcon size="1.4em" />
                      {t('actions.markPlain')}
                    </DropdownMenuItem>
                  ) : (
                    <DropdownMenuItem
                      onClick={() => {
                        props.onSensitivity(true)
                      }}
                    >
                      <ShieldCheckGlassIcon
                        className="text-tone-success"
                        size="1.4em"
                      />
                      {t('actions.markSensitive')}
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem
                    onClick={() => {
                      props.onSensitivity(null)
                    }}
                  >
                    {t('actions.useAutomatic')}
                  </DropdownMenuItem>
                  {props.canConvert ? (
                    <DropdownMenuItem onClick={props.onConvert}>
                      <LockGlassIcon size="1.4em" />
                      {t('actions.convert')}
                    </DropdownMenuItem>
                  ) : null}
                  <DropdownMenuSeparator />
                </>
              ) : null}
              <DropdownMenuItem
                onClick={() => {
                  props.onChange({ ...row, removed: true })
                }}
              >
                <TrashGlassIcon size="1.4em" />
                {t('actions.remove')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
    </div>
  )
}
