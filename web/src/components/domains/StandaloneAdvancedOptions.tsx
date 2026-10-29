import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { AdvancedDraft } from '../../lib/standaloneDomain'

function SwitchRow({
  id,
  label,
  hint,
  checked,
  disabled,
  onChange,
}: {
  id: string
  label: string
  hint?: string
  checked: boolean
  disabled?: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div>
        <label htmlFor={id} className="text-sm">
          {label}
        </label>
        {hint ? <FieldHint>{hint}</FieldHint> : null}
      </div>
      <Switch
        id={id}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onChange}
      />
    </div>
  )
}

export function StandaloneAdvancedOptions({
  value,
  urlTarget,
  onChange,
}: {
  value: AdvancedDraft
  urlTarget: boolean
  onChange: (next: AdvancedDraft) => void
}) {
  const { t } = useTranslation('domains')
  const set = (patch: Partial<AdvancedDraft>) => {
    onChange({ ...value, ...patch })
  }
  const tlsOn = urlTarget || value.upstreamTls
  return (
    <Collapsible>
      <CollapsibleTrigger className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ChevronDownGlassIcon className="size-4" aria-hidden="true" />
        {t('standalone.advanced')}
      </CollapsibleTrigger>
      <CollapsiblePanel className="space-y-3 pt-3">
        {urlTarget ? null : (
          <SwitchRow
            id="standalone-tls"
            label={t('standalone.upstreamTls')}
            checked={value.upstreamTls}
            onChange={(upstreamTls) => {
              set({ upstreamTls })
            }}
          />
        )}
        {tlsOn ? (
          <>
            <SwitchRow
              id="standalone-verify"
              label={t('standalone.verifyTls')}
              hint={t('standalone.verifyTlsHint')}
              checked={value.verifyTls}
              onChange={(verifyTls) => {
                set({ verifyTls })
              }}
            />
            <Field>
              <FieldLabel htmlFor="standalone-sni">
                {t('standalone.serverName')}
              </FieldLabel>
              <Input
                id="standalone-sni"
                className="font-mono"
                value={value.serverName}
                placeholder={t('standalone.serverNamePlaceholder')}
                onChange={(e) => {
                  set({ serverName: e.target.value.trim() })
                }}
              />
            </Field>
          </>
        ) : null}
        <Field>
          <FieldLabel htmlFor="standalone-host">
            {t('standalone.hostHeader')}
          </FieldLabel>
          <Select
            value={
              value.hostMode === 'default'
                ? urlTarget
                  ? 'upstream'
                  : 'preserve'
                : value.hostMode
            }
            onValueChange={(v) => {
              set({ hostMode: v ?? 'default' })
            }}
          >
            <SelectTrigger id="standalone-host" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="preserve">
                {t('standalone.hostPreserve')}
              </SelectItem>
              <SelectItem value="upstream">
                {t('standalone.hostUpstream')}
              </SelectItem>
              <SelectItem value="custom">
                {t('standalone.hostCustom')}
              </SelectItem>
            </SelectContent>
          </Select>
          {value.hostMode === 'custom' ? (
            <Input
              aria-label={t('standalone.hostCustom')}
              className="mt-1.5 font-mono"
              value={value.hostCustom}
              placeholder={t('standalone.hostCustomPlaceholder')}
              onChange={(e) => {
                set({ hostCustom: e.target.value.trim() })
              }}
            />
          ) : null}
        </Field>
        <SwitchRow
          id="standalone-ws"
          label={t('standalone.websocket')}
          checked={value.websocket}
          onChange={(websocket) => {
            set({ websocket })
          }}
        />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="standalone-connect">
              {t('standalone.connectTimeout')}
            </FieldLabel>
            <Input
              id="standalone-connect"
              inputMode="numeric"
              value={value.connectTimeout}
              placeholder="0"
              onChange={(e) => {
                set({ connectTimeout: e.target.value.replace(/[^0-9]/g, '') })
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="standalone-read">
              {t('standalone.readTimeout')}
            </FieldLabel>
            <Input
              id="standalone-read"
              inputMode="numeric"
              value={value.readTimeout}
              placeholder="0"
              onChange={(e) => {
                set({ readTimeout: e.target.value.replace(/[^0-9]/g, '') })
              }}
            />
          </Field>
        </div>
        <FieldHint>{t('standalone.timeoutHint')}</FieldHint>
        <Field>
          <FieldLabel htmlFor="standalone-strip">
            {t('standalone.stripPrefix')}
          </FieldLabel>
          <Input
            id="standalone-strip"
            className="font-mono"
            value={value.stripPrefix}
            placeholder={t('standalone.stripPrefixPlaceholder')}
            onChange={(e) => {
              set({ stripPrefix: e.target.value.trim() })
            }}
          />
        </Field>
      </CollapsiblePanel>
    </Collapsible>
  )
}
