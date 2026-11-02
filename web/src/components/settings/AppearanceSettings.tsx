import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { RotateCcwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { SettingRow, SettingRows } from '@/components/ui/setting-row'
import { ThemeChoice } from '@/components/account/ThemeChoice'
import { useTheme } from '@/components/ThemeProvider'
import {
  APPEARANCE_RECOMMENDED as REC,
  APPEARANCE_DEFAULTS as DEF,
  ACCENTS,
  DARK_BASES,
  FONTS,
  GUTTERS,
  LIGHT_BASES,
  PRESETS,
  TEXT_SIZES,
  applyPreset,
  resetAppearance,
  setAppearance,
  useAppearance,
  type PresetId,
} from '@/lib/appearance'
import { setDensity, useDensity, type Density } from '@/lib/density'
import { PAGE_WIDTHS, setPageWidth, usePageWidth } from '@/lib/pageWidth'
import { ChoiceGroup, type Choice } from './ChoiceGroup'
import { AppearancePreview } from './AppearancePreview'

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

/** AppearanceSettings is the one place for theme base, font, size, width, spacing and density. */
export function AppearanceSettings() {
  const { t } = useTranslation('settings')
  const { theme, setTheme } = useTheme()
  const appearance = useAppearance()
  const width = usePageWidth()
  const density = useDensity()
  const themeLabelId = useId()

  const common = {
    defaultLabel: t('appearancePage.defaultBadge'),
    recommendedLabel: t('appearancePage.recommendedBadge'),
  }

  const onPreset = (id: PresetId) => {
    const preset = applyPreset(id)
    if (preset.theme) setTheme(preset.theme)
  }

  const darkOptions: Choice<(typeof DARK_BASES)[number]>[] = DARK_BASES.map(
    (v) => ({
      value: v,
      label: t(`appearancePage.darkBase.${v}`),
      hint: t(`appearancePage.darkBase.${v}Hint`),
      sample: <BaseSample id={v} />,
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
  const sizeOptions: Choice<(typeof TEXT_SIZES)[number]>[] = TEXT_SIZES.map(
    (v) => ({
      value: v,
      label: `${v}%`,
      isDefault: v === DEF.textSize,
      isRecommended: v === REC.textSize,
    }),
  )
  const widthOptions: Choice<(typeof PAGE_WIDTHS)[number]>[] = PAGE_WIDTHS.map(
    (v) => ({
      value: v,
      label: t(`generalPage.width.${v}`),
      hint: t(`generalPage.width.${v}Hint`),
      isDefault: v === 'comfortable',
      isRecommended: v === REC.width,
    }),
  )
  const gutterOptions: Choice<(typeof GUTTERS)[number]>[] = GUTTERS.map(
    (v) => ({
      value: v,
      label: t(`appearancePage.gutter.${v}`),
      hint: t(`appearancePage.gutter.${v}Hint`),
      isDefault: v === DEF.gutter,
      isRecommended: v === REC.gutter,
    }),
  )
  const accentOptions: Choice<(typeof ACCENTS)[number]>[] = ACCENTS.map(
    (v) => ({
      value: v,
      label: t(`appearancePage.accent.${v}`),
      sample: <AccentSample id={v} />,
      isDefault: v === DEF.accent,
      isRecommended: v === REC.accent,
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

  return (
    <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <div className="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>{t('appearancePage.presets.title')}</CardTitle>
            <CardDescription>
              {t('appearancePage.presets.description')}
            </CardDescription>
            <CardAction>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={resetAppearance}
              >
                <RotateCcwGlassIcon size="1.4em" aria-hidden="true" />
                {t('appearancePage.reset')}
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent>
            <div
              role="group"
              aria-label={t('appearancePage.presets.title')}
              className="flex flex-wrap gap-2"
            >
              {PRESETS.map((id) => (
                <Button
                  key={id}
                  type="button"
                  size="sm"
                  variant="outline"
                  title={t(`appearancePage.presets.${id}Hint`)}
                  onClick={() => onPreset(id)}
                >
                  {t(`appearancePage.presets.${id}`)}
                </Button>
              ))}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent>
            <SettingRows>
              <SettingRow
                title={
                  <span id={themeLabelId}>
                    {t('appearancePage.theme.title')}
                  </span>
                }
                description={t('appearancePage.theme.description')}
              >
                <ThemeChoice
                  value={theme}
                  onChange={setTheme}
                  labelledBy={themeLabelId}
                />
              </SettingRow>
              <SettingRow
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
                title={t('appearancePage.accent.title')}
                description={t('appearancePage.accent.description')}
              >
                <ChoiceGroup
                  label={t('appearancePage.accent.title')}
                  value={appearance.accent}
                  options={accentOptions}
                  onChange={(accent) => setAppearance({ accent })}
                  className="sm:grid-cols-3"
                  {...common}
                />
              </SettingRow>
              <SettingRow
                title={t('appearancePage.font.title')}
                description={t('appearancePage.font.description')}
              >
                <ChoiceGroup
                  label={t('appearancePage.font.title')}
                  value={appearance.font}
                  options={fontOptions}
                  onChange={(font) => setAppearance({ font })}
                  className="sm:grid-cols-3"
                  {...common}
                />
              </SettingRow>
              <SettingRow
                title={t('appearancePage.textSize.title')}
                description={t('appearancePage.textSize.description')}
              >
                <ChoiceGroup
                  label={t('appearancePage.textSize.title')}
                  value={appearance.textSize}
                  options={sizeOptions}
                  onChange={(textSize) => setAppearance({ textSize })}
                  className="sm:grid-cols-3"
                  {...common}
                />
              </SettingRow>
              <SettingRow
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
              </SettingRow>
              <SettingRow
                title={t('appearancePage.gutter.title')}
                description={t('appearancePage.gutter.description')}
              >
                <ChoiceGroup
                  label={t('appearancePage.gutter.title')}
                  value={appearance.gutter}
                  options={gutterOptions}
                  onChange={(gutter) => setAppearance({ gutter })}
                  className="sm:grid-cols-3"
                  {...common}
                />
              </SettingRow>
              <SettingRow
                title={t('generalPage.density.title')}
                description={t('appearancePage.density.description')}
              >
                <ChoiceGroup
                  label={t('generalPage.density.title')}
                  value={density}
                  options={densityOptions}
                  onChange={setDensity}
                  className="sm:grid-cols-2"
                  {...common}
                />
              </SettingRow>
            </SettingRows>
          </CardContent>
        </Card>
        <p className="text-xs text-muted-foreground">
          {t('appearancePage.savedHere')}
        </p>
      </div>
      <div className="xl:sticky xl:top-4">
        <AppearancePreview />
      </div>
    </div>
  )
}
