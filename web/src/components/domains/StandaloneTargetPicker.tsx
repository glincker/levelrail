import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { TargetDraft, TargetMode } from '../../lib/standaloneDomain'
import { useStandaloneCandidates } from '../../queries/standaloneDomains'

const MODES: TargetMode[] = ['port', 'address', 'url', 'app', 'container']

function Chips({
  values,
  selected,
  onPick,
}: {
  values: { value: string; label: string; disabled?: boolean }[]
  selected: string
  onPick: (value: string) => void
}) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {values.map((v) => (
        <Button
          key={v.value}
          type="button"
          size="xs"
          variant={selected === v.value ? 'secondary' : 'outline'}
          disabled={v.disabled}
          title={v.label}
          onClick={() => {
            onPick(v.value)
          }}
        >
          <span className="font-mono">{v.value}</span>
          {v.label !== v.value ? (
            <span className="text-muted-foreground">{v.label}</span>
          ) : null}
        </Button>
      ))}
    </div>
  )
}

export function StandaloneTargetPicker({
  draft,
  onChange,
}: {
  draft: TargetDraft
  onChange: (next: TargetDraft) => void
}) {
  const { t } = useTranslation('domains')
  const candidates = useStandaloneCandidates(true)
  const set = (patch: Partial<TargetDraft>) => {
    onChange({ ...draft, ...patch })
  }
  const hostPorts = candidates.data?.host_ports ?? []
  const apps = candidates.data?.apps ?? []
  const containers = candidates.data?.containers ?? []
  const appPort = apps.find((a) => a.name === draft.app)?.port
  const containerPorts =
    containers.find((c) => c.name === draft.container)?.ports ?? []

  return (
    <div className="space-y-3">
      <div
        role="radiogroup"
        aria-label={t('standalone.target')}
        className="flex flex-wrap gap-1.5"
      >
        {MODES.map((m) => (
          <Button
            key={m}
            type="button"
            size="sm"
            role="radio"
            aria-checked={draft.mode === m}
            variant={draft.mode === m ? 'secondary' : 'outline'}
            onClick={() => {
              set({ mode: m })
            }}
          >
            {t(`standalone.targetMode.${m}`)}
          </Button>
        ))}
      </div>

      {draft.mode === 'port' ? (
        <Field>
          <FieldLabel htmlFor="standalone-port">
            {t('standalone.port')}
          </FieldLabel>
          <Input
            id="standalone-port"
            inputMode="numeric"
            className="w-32 font-mono"
            value={draft.port}
            placeholder={t('standalone.portPlaceholder')}
            onChange={(e) => {
              set({ port: e.target.value.replace(/[^0-9]/g, '') })
            }}
          />
          <FieldHint>
            {hostPorts.length > 0
              ? t('standalone.listening')
              : t('standalone.listeningNone')}
          </FieldHint>
          {hostPorts.length > 0 ? (
            <Chips
              selected={draft.port}
              values={hostPorts.map((p) => ({
                value: String(p.port),
                label: p.label
                  ? t('standalone.controlPort', { label: p.label })
                  : String(p.port),
                disabled: p.blocked,
              }))}
              onPick={(port) => {
                set({ port })
              }}
            />
          ) : null}
        </Field>
      ) : null}

      {draft.mode === 'address' ? (
        <Field>
          <FieldLabel htmlFor="standalone-address">
            {t('standalone.address')}
          </FieldLabel>
          <Input
            id="standalone-address"
            className="font-mono"
            value={draft.address}
            placeholder={t('standalone.addressPlaceholder')}
            onChange={(e) => {
              set({ address: e.target.value.trim() })
            }}
          />
          <FieldHint>{t('standalone.addressHint')}</FieldHint>
        </Field>
      ) : null}

      {draft.mode === 'url' ? (
        <Field>
          <FieldLabel htmlFor="standalone-url">
            {t('standalone.url')}
          </FieldLabel>
          <Input
            id="standalone-url"
            type="url"
            className="font-mono"
            value={draft.url}
            placeholder={t('standalone.urlPlaceholder')}
            onChange={(e) => {
              set({ url: e.target.value.trim() })
            }}
          />
          <FieldHint>{t('standalone.urlHint')}</FieldHint>
        </Field>
      ) : null}

      {draft.mode === 'app' ? (
        <div className="grid gap-3 sm:grid-cols-[1fr_8rem]">
          <Field>
            <FieldLabel htmlFor="standalone-app">
              {t('standalone.app')}
            </FieldLabel>
            <Select
              value={draft.app || null}
              onValueChange={(v) => {
                set({ app: v ?? '' })
              }}
            >
              <SelectTrigger id="standalone-app" className="w-full">
                <SelectValue placeholder={t('standalone.appPlaceholder')} />
              </SelectTrigger>
              <SelectContent>
                {apps.map((a) => (
                  <SelectItem key={a.name} value={a.name}>
                    {a.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="standalone-app-port">
              {t('standalone.appPort')}
            </FieldLabel>
            <Input
              id="standalone-app-port"
              inputMode="numeric"
              className="font-mono"
              value={draft.appPort}
              placeholder={appPort ? String(appPort) : ''}
              onChange={(e) => {
                set({ appPort: e.target.value.replace(/[^0-9]/g, '') })
              }}
            />
          </Field>
          {appPort ? (
            <FieldHint className="sm:col-span-2">
              {t('standalone.appPortHint', { port: appPort })}
            </FieldHint>
          ) : null}
        </div>
      ) : null}

      {draft.mode === 'container' ? (
        <div className="space-y-2">
          <div className="grid gap-3 sm:grid-cols-[1fr_8rem]">
            <Field>
              <FieldLabel htmlFor="standalone-container">
                {t('standalone.container')}
              </FieldLabel>
              <Select
                value={draft.container || null}
                onValueChange={(v) => {
                  set({ container: v ?? '' })
                }}
              >
                <SelectTrigger id="standalone-container" className="w-full">
                  <SelectValue
                    placeholder={t('standalone.containerPlaceholder')}
                  />
                </SelectTrigger>
                <SelectContent>
                  {containers.map((c) => (
                    <SelectItem key={c.name} value={c.name}>
                      {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel htmlFor="standalone-container-port">
                {t('standalone.containerPort')}
              </FieldLabel>
              <Input
                id="standalone-container-port"
                inputMode="numeric"
                className="font-mono"
                value={draft.containerPort}
                onChange={(e) => {
                  set({ containerPort: e.target.value.replace(/[^0-9]/g, '') })
                }}
              />
            </Field>
          </div>
          {containers.length === 0 && !candidates.isLoading ? (
            <FieldHint>{t('standalone.noContainers')}</FieldHint>
          ) : null}
          {containerPorts.length > 0 ? (
            <Chips
              selected={draft.containerPort}
              values={containerPorts.map((p) => ({
                value: String(p),
                label: String(p),
              }))}
              onPick={(containerPort) => {
                set({ containerPort })
              }}
            />
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
