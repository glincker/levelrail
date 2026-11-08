import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BookOpenGlassIcon,
  ContrastGlassIcon,
  LayoutDashboardGlassIcon,
  MoonGlassIcon,
  PaletteGlassIcon,
  RefreshCwGlassIcon,
  SparklesGlassIcon,
  TypeGlassIcon,
} from '@thesvg/ui-react'
import type { ComponentType, ReactNode } from 'react'
import { SettingsLayout, SettingsSection } from '@/components/kit'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  SelectedMark,
  selectableCardClass,
} from '@/components/ui/selectable-card'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { SettingRow, SettingRows } from '@/components/ui/setting-row'
import { AppearanceHub } from './appearance/AppearanceHub'
import { useTheme } from '@/components/ThemeProvider'
import {
  APPEARANCE_RECOMMENDED as REC,
  APPEARANCE_DEFAULTS as DEF,
  ACCENT_CHOICES,
  CODE_FONTS,
  CODE_SIZE_MAX,
  CODE_SIZE_MIN,
  CONTRASTS,
  RADII,
  LOOKS,
  clampCodeSize,
  clampContentWidth,
  DARK_BASES,
  FONTS,
  GUTTERS,
  LIGHT_BASES,
  PRESETS,
  PRESET_VALUES,
  MOTIONS,
  TEXT_SIZE_MAX,
  TEXT_SIZE_MIN,
  TEXT_SIZE_QUICK,
  TEXT_SIZE_STEP,
  applyPreset,
  clampTextSize,
  customAccent,
  resetAppearance,
  setAppearance,
  useAppearance,
  type Appearance,
  type PresetId,
} from '@/lib/appearance'
import { normalizeHex } from '@/lib/accentContrast'
import { setDensity, useDensity, type Density } from '@/lib/density'
import {
  CONTENT_WIDTH_MAX,
  CONTENT_WIDTH_MIN,
  CONTENT_WIDTH_STEP,
  PAGE_WIDTH_CHOICES,
  setPageWidth,
  usePageWidth,
} from '@/lib/pageWidth'
import { cn } from '@/lib/utils'
import { ChoiceGroup, type Choice } from './ChoiceGroup'
import { AppearanceSyncRow } from './AppearanceSyncRow'
import { appearanceSync } from '@/queries/appearanceSync'

// Static sample colors mirror the index.css base blocks: background, card, border.
const BASE_SAMPLE: Record<string, string> = {
  soft: 'bg-[oklch(0.145_0_0)] [--c:oklch(0.205_0_0)] [--b:oklch(0.3_0_0)]',
  black: 'bg-[oklch(0_0_0)] [--c:oklch(0.15_0_0)] [--b:oklch(0.32_0_0)]',
  graphite:
    'bg-[oklch(0.2_0.006_260)] [--c:oklch(0.255_0.007_260)] [--b:oklch(0.37_0.008_260)]',
  midnight:
    'bg-[oklch(0.16_0.035_265)] [--c:oklch(0.21_0.04_265)] [--b:oklch(0.34_0.04_265)]',
  white: 'bg-[oklch(1_0_0)] [--c:oklch(0.97_0_0)] [--b:oklch(0.922_0_0)]',
  grey: 'bg-[oklch(0.955_0_0)] [--c:oklch(0.99_0_0)] [--b:oklch(0.86_0_0)]',
  paper:
    'bg-[oklch(0.965_0.016_85)] [--c:oklch(0.985_0.01_85)] [--b:oklch(0.875_0.02_85)]',
  cool: 'bg-[oklch(0.95_0.01_250)] [--c:oklch(0.98_0.006_250)] [--b:oklch(0.86_0.014_250)]',
}

const ACCENT_SAMPLE: Record<string, string> = {
  teal: 'bg-[oklch(0.52_0.115_215)] dark:bg-[oklch(0.76_0.115_215)]',
  indigo: 'bg-[oklch(0.52_0.16_275)] dark:bg-[oklch(0.76_0.16_275)]',
  violet: 'bg-[oklch(0.52_0.16_300)] dark:bg-[oklch(0.76_0.16_300)]',
  green: 'bg-[oklch(0.52_0.13_155)] dark:bg-[oklch(0.76_0.13_155)]',
  amber: 'bg-[oklch(0.52_0.14_70)] dark:bg-[oklch(0.76_0.14_70)]',
  rose: 'bg-[oklch(0.52_0.17_10)] dark:bg-[oklch(0.76_0.17_10)]',
  custom:
    'bg-[conic-gradient(from_0deg,oklch(0.7_0.15_30),oklch(0.8_0.15_100),oklch(0.7_0.15_160),oklch(0.7_0.15_240),oklch(0.7_0.15_320),oklch(0.7_0.15_30))]',
}

function AccentSample({ id }: { id: string }) {
  return (
    <span
      aria-hidden="true"
      className={`size-5 rounded-full ${ACCENT_SAMPLE[id] ?? ''}`}
    />
  )
}

function BaseSample({ id }: { id: string }) {
  return (
    <span
      aria-hidden="true"
      className={`flex h-9 w-full items-center justify-center rounded-md border border-(--b) ${BASE_SAMPLE[id] ?? ''}`}
    >
      <span className="h-5 w-3/5 rounded-sm border border-(--b) bg-(--c)" />
    </span>
  )
}

const FONT_SAMPLE: Record<string, string> = {
  system: 'font-sans',
  device: 'font-[family-name:ui-sans-serif,system-ui,sans-serif]',
  rounded: 'font-[family-name:ui-rounded,system-ui,sans-serif]',
  serif: 'font-serif',
  mono: 'font-mono',
  readable: 'font-[family-name:Verdana,Segoe_UI,sans-serif]',
}

function FontSample({ id }: { id: string }) {
  return (
    <span
      aria-hidden="true"
      className={`text-xl leading-none text-foreground ${FONT_SAMPLE[id] ?? ''}`}
    >
      Aa
    </span>
  )
}

const CODE_FONT_SAMPLE: Record<string, string> = {
  'system-mono': 'font-mono',
  'device-mono': 'font-[family-name:ui-monospace,Menlo,Consolas,monospace]',
  jetbrains: "font-[family-name:'JetBrains_Mono',ui-monospace,monospace]",
  fira: "font-[family-name:'Fira_Code',ui-monospace,monospace]",
  menlo: 'font-[family-name:Menlo,Monaco,ui-monospace,monospace]',
}

function CodeFontSample({ id }: { id: string }) {
  return (
    <span
      aria-hidden="true"
      className={`text-sm leading-none text-foreground ${CODE_FONT_SAMPLE[id] ?? ''}`}
    >
      {'{ 0O 1l }'}
    </span>
  )
}

const RADIUS_SAMPLE: Record<string, string> = {
  sharp: 'rounded-tl-none',
  normal: 'rounded-tl-lg',
  soft: 'rounded-tl-3xl',
}

function RadiusSample({ id }: { id: string }) {
  return (
    <span
      aria-hidden="true"
      className={`block size-6 border-t-2 border-l-2 border-state-accent ${RADIUS_SAMPLE[id] ?? ''}`}
    />
  )
}

const PRESET_ICON: Record<
  PresetId,
  ComponentType<{ className?: string; 'aria-hidden'?: boolean | 'true' }>
> = {
  default: SparklesGlassIcon,
  focusDark: MoonGlassIcon,
  oled: ContrastGlassIcon,
  paper: BookOpenGlassIcon,
}

/** activePreset returns the preset matching every current value, or null for a custom mix. */
function activePreset(
  appearance: Appearance,
  width: string,
  density: string,
  theme: string,
): PresetId | null {
  return (
    PRESETS.find((id) => {
      const p = PRESET_VALUES[id]
      if (p.width !== width || p.density !== density) return false
      if (p.theme && p.theme !== theme) return false
      return (Object.keys(p.appearance) as (keyof Appearance)[]).every(
        (k) =>
          // The stored custom hex only matters while the custom accent is on.
          (k === 'accentCustom' && appearance.accent !== 'custom') ||
          p.appearance[k] === appearance[k],
      )
    }) ?? null
  )
}

function PresetSwatch({ id }: { id: PresetId }) {
  const p = PRESET_VALUES[id]
  const base =
    p.theme === 'light' ? p.appearance.lightBase : p.appearance.darkBase
  return (
    <span className="flex items-center gap-2" aria-hidden="true">
      <span className="min-w-0 flex-1">
        <BaseSample id={base} />
      </span>
      <AccentSample id={p.appearance.accent} />
    </span>
  )
}

function TextSizeControl({
  value,
  label,
  quickLabel,
  sample,
  resetLabel,
}: {
  value: number
  label: string
  quickLabel: string
  sample: string
  resetLabel: string
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-3">
        <input
          type="range"
          aria-label={label}
          min={TEXT_SIZE_MIN}
          max={TEXT_SIZE_MAX}
          step={TEXT_SIZE_STEP}
          value={value}
          onChange={(e) =>
            setAppearance({ textSize: clampTextSize(e.target.value) })
          }
          className="h-6 min-w-0 flex-1 cursor-pointer accent-state-accent"
        />
        <output className="w-12 text-right text-sm font-medium tabular-nums">
          {value}%
        </output>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <div
          role="group"
          aria-label={quickLabel}
          className="flex items-center gap-1.5"
        >
          {TEXT_SIZE_QUICK.map((n) => (
            <Button
              key={n}
              type="button"
              size="sm"
              variant="outline"
              aria-pressed={value === n}
              className={cn(value === n && 'border-state-accent')}
              onClick={() => setAppearance({ textSize: n })}
            >
              {n}%
            </Button>
          ))}
        </div>
        {value !== DEF.textSize ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => setAppearance({ textSize: DEF.textSize })}
          >
            {resetLabel}
          </Button>
        ) : null}
        <span className="ml-auto text-foreground" aria-hidden="true">
          {sample}
        </span>
      </div>
    </div>
  )
}

function SliderControl({
  value,
  label,
  min,
  max,
  step,
  unit,
  resetLabel,
  isDefault,
  onChange,
  onReset,
}: {
  value: number
  label: string
  min: number
  max: number
  step: number
  unit: string
  resetLabel: string
  isDefault: boolean
  onChange: (next: string) => void
  onReset: () => void
}) {
  return (
    <div className="flex items-center gap-3">
      <input
        type="range"
        aria-label={label}
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-6 min-w-0 flex-1 cursor-pointer accent-state-accent"
      />
      <output className="w-16 text-right text-sm font-medium tabular-nums">
        {value}
        {unit}
      </output>
      {isDefault ? null : (
        <Button type="button" size="sm" variant="ghost" onClick={onReset}>
          {resetLabel}
        </Button>
      )}
    </div>
  )
}

function CustomAccentEditor({ appearance }: { appearance: Appearance }) {
  const { t } = useTranslation('settings')
  const [draft, setDraft] = useState<string | null>(null)
  const hexId = useId()
  const shown = draft ?? appearance.accentCustom
  const invalid = draft !== null && normalizeHex(draft) === null
  const adjusted = customAccent(appearance).adjusted

  const commit = (hex: string) => {
    setAppearance({ accent: 'custom', accentCustom: hex })
  }

  return (
    <div className="mt-2 space-y-1.5">
      <div className="flex items-center gap-2">
        <input
          type="color"
          aria-label={t('appearancePage.accent.customColor')}
          value={appearance.accentCustom}
          onChange={(e) => {
            setDraft(null)
            commit(e.target.value)
          }}
          className="size-8 shrink-0 cursor-pointer rounded-md border border-input bg-transparent p-0.5"
        />
        <Input
          id={hexId}
          aria-label={t('appearancePage.accent.customHex')}
          aria-invalid={invalid}
          spellCheck={false}
          maxLength={7}
          value={shown}
          onChange={(e) => {
            const next = e.target.value
            const hex = normalizeHex(next)
            if (hex) {
              setDraft(null)
              commit(hex)
            } else {
              setDraft(next)
            }
          }}
          onBlur={() => setDraft(null)}
          className="w-28 font-mono"
        />
      </div>
      {invalid ? (
        <p role="alert" className="text-xs text-destructive">
          {t('appearancePage.accent.customInvalid')}
        </p>
      ) : null}
      {adjusted ? (
        <p role="status" className="text-xs text-muted-foreground">
          {t('appearancePage.accent.adjusted')}
        </p>
      ) : null}
    </div>
  )
}

function SectionCard({
  icon,
  title,
  children,
}: {
  icon: ReactNode
  title: string
  children: ReactNode
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {icon}
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <SettingRows>{children}</SettingRows>
      </CardContent>
    </Card>
  )
}

// Stack title over control: the section cards are only half width from xl.
const ROW = 'md:grid-cols-1 lg:grid-cols-1 py-3'
const ROW_PAIR = 'md:grid-cols-1 lg:grid-cols-1 py-0'

/** AppearanceSettings is the one place for theme base, font, size, width, spacing and density. */
export function AppearanceSettings() {
  const { t } = useTranslation('settings')
  const { theme, setTheme } = useTheme()
  const appearance = useAppearance()
  const width = usePageWidth()
  const density = useDensity()

  const common = {
    defaultLabel: t('appearancePage.defaultBadge'),
    recommendedLabel: t('appearancePage.recommendedBadge'),
  }

  const onPreset = (id: PresetId) => {
    const preset = applyPreset(id)
    if (preset.theme) setTheme(preset.theme)
  }

  const onReset = () => {
    resetAppearance()
    void appearanceSync.clearAccount()
  }

  const darkOptions: Choice<(typeof DARK_BASES)[number]>[] = DARK_BASES.map(
    (v) => ({
      value: v,
      label: t(`appearancePage.darkBase.${v}`),
      hint: t(`appearancePage.darkBase.${v}Hint`),
      sample: <BaseSample id={v} />,
      block: true,
      isDefault: v === DEF.darkBase,
      isRecommended: v === REC.darkBase,
    }),
  )
  const lightOptions: Choice<(typeof LIGHT_BASES)[number]>[] = LIGHT_BASES.map(
    (v) => ({
      value: v,
      label: t(`appearancePage.lightBase.${v}`),
      hint: t(`appearancePage.lightBase.${v}Hint`),
      sample: <BaseSample id={v} />,
      block: true,
      isDefault: v === DEF.lightBase,
      isRecommended: v === REC.lightBase,
    }),
  )
  const fontOptions: Choice<(typeof FONTS)[number]>[] = FONTS.map((v) => ({
    value: v,
    label: t(`appearancePage.font.${v}`),
    hint: t(`appearancePage.font.${v}Hint`),
    sample: <FontSample id={v} />,
    isDefault: v === DEF.font,
    isRecommended: v === REC.font,
  }))
  const widthOptions: Choice<(typeof PAGE_WIDTH_CHOICES)[number]>[] =
    PAGE_WIDTH_CHOICES.map((v) => ({
      value: v,
      label: t(`generalPage.width.${v}`),
      hint: t(`generalPage.width.${v}Hint`),
      isDefault: v === 'comfortable',
      isRecommended: v === REC.width,
    }))
  const gutterOptions: Choice<(typeof GUTTERS)[number]>[] = GUTTERS.map(
    (v) => ({
      value: v,
      label: t(`appearancePage.gutter.${v}`),
      hint: t(`appearancePage.gutter.${v}Hint`),
      isDefault: v === DEF.gutter,
      isRecommended: v === REC.gutter,
    }),
  )
  const accentOptions: Choice<(typeof ACCENT_CHOICES)[number]>[] =
    ACCENT_CHOICES.map((v) => ({
      value: v,
      label: t(`appearancePage.accent.${v}`),
      sample: <AccentSample id={v} />,
      isDefault: v === DEF.accent,
      isRecommended: v === REC.accent,
    }))
  const radiusOptions: Choice<(typeof RADII)[number]>[] = RADII.map((v) => ({
    value: v,
    label: t(`appearancePage.radius.${v}`),
    hint: t(`appearancePage.radius.${v}Hint`),
    sample: <RadiusSample id={v} />,
    isDefault: v === DEF.radius,
  }))
  const lookOptions: Choice<(typeof LOOKS)[number]>[] = LOOKS.map((v) => ({
    value: v,
    label: t(`appearancePage.look.${v}`),
    hint: t(`appearancePage.look.${v}Hint`),
    isDefault: v === DEF.look,
  }))
  const contrastOptions: Choice<(typeof CONTRASTS)[number]>[] = CONTRASTS.map(
    (v) => ({
      value: v,
      label: t(`appearancePage.contrast.${v}`),
      hint: t(`appearancePage.contrast.${v}Hint`),
      isDefault: v === DEF.contrast,
    }),
  )
  const codeFontOptions: Choice<(typeof CODE_FONTS)[number]>[] = CODE_FONTS.map(
    (v) => ({
      value: v,
      label: t(`appearancePage.codeFont.${v}`),
      hint: t(`appearancePage.codeFont.${v}Hint`),
      sample: <CodeFontSample id={v} />,
      isDefault: v === DEF.codeFont,
    }),
  )
  const motionOptions: Choice<(typeof MOTIONS)[number]>[] = MOTIONS.map(
    (v) => ({
      value: v,
      label: t(`appearancePage.motion.${v}`),
      hint: t(`appearancePage.motion.${v}Hint`),
      isDefault: v === DEF.motion,
    }),
  )
  const densityOptions: Choice<Density>[] = (
    ['compact', 'comfortable'] as const
  ).map((v) => ({
    value: v,
    label: t(`generalPage.density.${v}`),
    hint: t(`appearancePage.density.${v}Hint`),
    isDefault: v === 'compact',
    isRecommended: v === REC.density,
  }))

  const current = activePreset(appearance, width, density, theme)

  const sections = [
    {
      id: 'presets',
      label: t('appearancePage.presets.title'),
      icon: <SparklesGlassIcon size="1.1em" />,
    },
    {
      id: 'color',
      label: t('appearancePage.sections.color'),
      icon: <PaletteGlassIcon size="1.1em" />,
    },
    {
      id: 'type',
      label: t('appearancePage.sections.type'),
      icon: <TypeGlassIcon size="1.1em" />,
    },
    {
      id: 'layout',
      label: t('appearancePage.sections.layout'),
      icon: <LayoutDashboardGlassIcon size="1.1em" />,
    },
    {
      id: 'sync',
      label: t('appearancePage.sections.sync'),
      icon: <RefreshCwGlassIcon size="1.1em" />,
    },
  ]

  return (
    <SettingsLayout sections={sections}>
      <AppearanceHub onReset={onReset} />
      <SettingsSection
        id="presets"
        title={t('appearancePage.presets.title')}
        hideHeading
        columns={1}
      >
        <Card>
          <CardHeader>
            <CardTitle>{t('appearancePage.presets.title')}</CardTitle>
            <CardDescription>
              {current
                ? t('appearancePage.presets.description')
                : t('appearancePage.presets.customDescription')}
            </CardDescription>
            <CardAction className="flex items-center gap-2">
              {current ? null : (
                <Badge variant="accent">
                  {t('appearancePage.presets.custom')}
                </Badge>
              )}
            </CardAction>
          </CardHeader>
          <CardContent>
            <div
              role="group"
              aria-label={t('appearancePage.presets.title')}
              className="grid grid-cols-2 gap-2 lg:grid-cols-4"
            >
              {PRESETS.map((id) => {
                const Icon = PRESET_ICON[id]
                const active = current === id
                return (
                  <button
                    key={id}
                    type="button"
                    aria-pressed={active}
                    onClick={() => onPreset(id)}
                    className={cn(
                      selectableCardClass(
                        active,
                        'flex min-w-0 flex-col items-start gap-2 rounded-lg p-3 text-left',
                      ),
                      active && 'ring-1 ring-state-accent',
                    )}
                  >
                    <SelectedMark selected={active} />
                    <span className="flex items-center gap-2 text-sm font-medium">
                      <Icon
                        aria-hidden="true"
                        className="size-4 text-state-accent"
                      />
                      {t(`appearancePage.presets.${id}`)}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {t(`appearancePage.presets.${id}Hint`)}
                    </span>
                    <span className="w-full">
                      <PresetSwatch id={id} />
                    </span>
                  </button>
                )
              })}
            </div>
          </CardContent>
        </Card>
      </SettingsSection>

      <SettingsSection
        id="color"
        title={t('appearancePage.sections.color')}
        hideHeading
        columns={1}
      >
        <SectionCard
          icon={
            <PaletteGlassIcon
              className="size-4 text-state-accent"
              aria-hidden="true"
            />
          }
          title={t('appearancePage.sections.color')}
        >
          <SettingRow
            wide
            className={ROW}
            title={t('appearancePage.darkBase.title')}
            description={t('appearancePage.darkBase.description')}
          >
            <ChoiceGroup
              label={t('appearancePage.darkBase.title')}
              value={appearance.darkBase}
              options={darkOptions}
              onChange={(darkBase) => setAppearance({ darkBase })}
              {...common}
            />
          </SettingRow>
          <SettingRow
            wide
            className={ROW}
            title={t('appearancePage.lightBase.title')}
            description={t('appearancePage.lightBase.description')}
          >
            <ChoiceGroup
              label={t('appearancePage.lightBase.title')}
              value={appearance.lightBase}
              options={lightOptions}
              onChange={(lightBase) => setAppearance({ lightBase })}
              {...common}
            />
          </SettingRow>
          <SettingRow
            wide
            className={ROW}
            title={t('appearancePage.accent.title')}
            description={t('appearancePage.accent.description')}
          >
            <ChoiceGroup
              label={t('appearancePage.accent.title')}
              value={appearance.accent}
              options={accentOptions}
              onChange={(accent) => setAppearance({ accent })}
              {...common}
            />
            {appearance.accent === 'custom' ? (
              <CustomAccentEditor appearance={appearance} />
            ) : null}
          </SettingRow>
          <SettingRow
            wide
            className={ROW}
            title={t('appearancePage.contrast.title')}
            description={t('appearancePage.contrast.description')}
          >
            <ChoiceGroup
              label={t('appearancePage.contrast.title')}
              value={appearance.contrast}
              options={contrastOptions}
              onChange={(contrast) => setAppearance({ contrast })}
              {...common}
            />
            <p className="mt-2 rounded-md border border-border px-3 py-2 text-sm text-muted-foreground">
              {t('appearancePage.contrast.sample')}
            </p>
          </SettingRow>
        </SectionCard>
      </SettingsSection>

      <SettingsSection
        id="type"
        title={t('appearancePage.sections.type')}
        hideHeading
        columns={1}
      >
        <SectionCard
          icon={
            <TypeGlassIcon
              className="size-4 text-state-accent"
              aria-hidden="true"
            />
          }
          title={t('appearancePage.sections.type')}
        >
          <SettingRow
            wide
            className={ROW}
            title={t('appearancePage.font.title')}
            description={t('appearancePage.font.description')}
          >
            <ChoiceGroup
              label={t('appearancePage.font.title')}
              value={appearance.font}
              options={fontOptions}
              onChange={(font) => setAppearance({ font })}
              {...common}
            />
          </SettingRow>
          <SettingRow
            className={ROW}
            title={t('appearancePage.textSize.title')}
            description={t('appearancePage.textSize.description')}
          >
            <TextSizeControl
              value={appearance.textSize}
              label={t('appearancePage.textSize.slider')}
              quickLabel={t('appearancePage.textSize.quick')}
              sample={t('appearancePage.textSize.sample')}
              resetLabel={t('appearancePage.textSize.reset')}
            />
          </SettingRow>
          <SettingRow
            wide
            className={ROW}
            title={t('appearancePage.codeFont.title')}
            description={t('appearancePage.codeFont.description')}
          >
            <ChoiceGroup
              label={t('appearancePage.codeFont.title')}
              value={appearance.codeFont}
              options={codeFontOptions}
              onChange={(codeFont) => setAppearance({ codeFont })}
              {...common}
            />
          </SettingRow>
          <SettingRow
            className={ROW}
            title={t('appearancePage.codeSize.title')}
            description={t('appearancePage.codeSize.description')}
          >
            <div className="space-y-2">
              <SliderControl
                value={appearance.codeSize}
                label={t('appearancePage.codeSize.slider')}
                min={CODE_SIZE_MIN}
                max={CODE_SIZE_MAX}
                step={1}
                unit="px"
                resetLabel={t('appearancePage.codeSize.reset')}
                isDefault={appearance.codeSize === DEF.codeSize}
                onChange={(v) => setAppearance({ codeSize: clampCodeSize(v) })}
                onReset={() => setAppearance({ codeSize: DEF.codeSize })}
              />
              <pre
                aria-label={t('appearancePage.codeSize.sampleLabel')}
                className="overflow-x-auto rounded-md border border-border bg-muted px-3 py-2 font-mono"
              >
                {t('appearancePage.codeSize.sample')}
              </pre>
            </div>
          </SettingRow>
        </SectionCard>
      </SettingsSection>

      <SettingsSection
        id="layout"
        title={t('appearancePage.sections.layout')}
        hideHeading
        columns={1}
      >
        <SectionCard
          icon={
            <LayoutDashboardGlassIcon
              className="size-4 text-state-accent"
              aria-hidden="true"
            />
          }
          title={t('appearancePage.sections.layout')}
        >
          <SettingRow
            wide
            className={ROW}
            title={t('appearancePage.width.title')}
            description={t('appearancePage.width.description')}
          >
            <ChoiceGroup
              label={t('appearancePage.width.title')}
              value={width}
              options={widthOptions}
              onChange={setPageWidth}
              {...common}
            />
            {width === 'custom' ? (
              <div className="mt-2">
                <SliderControl
                  value={appearance.contentWidth}
                  label={t('appearancePage.width.slider')}
                  min={CONTENT_WIDTH_MIN}
                  max={CONTENT_WIDTH_MAX}
                  step={CONTENT_WIDTH_STEP}
                  unit="px"
                  resetLabel={t('appearancePage.width.reset')}
                  isDefault={appearance.contentWidth === DEF.contentWidth}
                  onChange={(v) =>
                    setAppearance({ contentWidth: clampContentWidth(v) })
                  }
                  onReset={() =>
                    setAppearance({ contentWidth: DEF.contentWidth })
                  }
                />
              </div>
            ) : null}
          </SettingRow>
          <SettingRow
            wide
            title={t('appearancePage.look.title')}
            description={t('appearancePage.look.description')}
          >
            <ChoiceGroup
              compact
              label={t('appearancePage.look.title')}
              value={appearance.look}
              options={lookOptions}
              onChange={(look) => setAppearance({ look })}
              {...common}
            />
          </SettingRow>
          <div className="grid gap-x-8 gap-y-4 py-4 xl:grid-cols-2">
            <SettingRow
              wide
              className={ROW_PAIR}
              title={t('appearancePage.radius.title')}
              description={t('appearancePage.radius.description')}
            >
              <ChoiceGroup
                compact
                label={t('appearancePage.radius.title')}
                value={appearance.radius}
                options={radiusOptions}
                onChange={(radius) => setAppearance({ radius })}
                {...common}
              />
            </SettingRow>
            <SettingRow
              wide
              className={ROW_PAIR}
              title={t('appearancePage.gutter.title')}
              description={t('appearancePage.gutter.description')}
            >
              <ChoiceGroup
                compact
                label={t('appearancePage.gutter.title')}
                value={appearance.gutter}
                options={gutterOptions}
                onChange={(gutter) => setAppearance({ gutter })}
                {...common}
              />
            </SettingRow>
          </div>
          <div className="grid gap-x-8 gap-y-4 py-4 xl:grid-cols-2">
            <SettingRow
              wide
              className={ROW_PAIR}
              title={t('generalPage.density.title')}
              description={t('appearancePage.density.description')}
            >
              <ChoiceGroup
                compact
                label={t('generalPage.density.title')}
                value={density}
                options={densityOptions}
                onChange={setDensity}
                {...common}
              />
            </SettingRow>
            <SettingRow
              wide
              className={ROW_PAIR}
              title={t('appearancePage.motion.title')}
              description={t('appearancePage.motion.description')}
            >
              <ChoiceGroup
                compact
                label={t('appearancePage.motion.title')}
                value={appearance.motion}
                options={motionOptions}
                onChange={(motion) => setAppearance({ motion })}
                {...common}
              />
            </SettingRow>
          </div>
        </SectionCard>
      </SettingsSection>

      <SettingsSection
        id="sync"
        title={t('appearancePage.sections.sync')}
        hideHeading
        columns={1}
      >
        <AppearanceSyncRow />
        <p className="text-xs text-muted-foreground">
          {t('appearancePage.savedHere')}
        </p>
      </SettingsSection>
    </SettingsLayout>
  )
}
