import { useTranslation } from 'react-i18next'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { BrandIcon, type BrandIconName } from '@/components/BrandIcon'
import { isBuildTypeValue } from '../lib/gitSourceForm'
import type { GitSourceBuildType } from '../types/gitSource'

// Frameworks/languages the auto-detect build pack actually supports,
// mirroring internal/build/railpack.go's supportedRailpackProviders.
const AUTO_DETECT_STACKS: { icon: BrandIconName; label: string }[] = [
  { icon: 'node', label: 'Node.js (React, Next.js, ...)' },
  { icon: 'go', label: 'Go' },
  { icon: 'java', label: 'Java (Spring Boot)' },
]

export function GitSourceBuildPackField({
  buildType,
  buildPath,
  disabled,
  onBuildType,
  onBuildPath,
}: {
  buildType: GitSourceBuildType
  buildPath: string
  disabled: boolean
  onBuildType: (v: GitSourceBuildType) => void
  onBuildPath: (v: string) => void
}) {
  const { t } = useTranslation('gitSource')
  return (
    <Field>
      <FieldLabel htmlFor="git-source-build-type">
        {t('buildPack.label')}
      </FieldLabel>
      <Tabs
        value={buildType}
        onValueChange={(v: unknown) => {
          if (isBuildTypeValue(v)) onBuildType(v)
        }}
      >
        <TabsList
          id="git-source-build-type"
          className="grid w-full grid-cols-3"
        >
          <TabsTrigger value="railpack" disabled={disabled}>
            {t('buildPack.autoDetect')}
          </TabsTrigger>
          <TabsTrigger value="dockerfile" disabled={disabled}>
            {t('buildPack.dockerfile')}
          </TabsTrigger>
          <TabsTrigger value="static" disabled={disabled}>
            {t('buildPack.static')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="railpack" className="space-y-2 pt-2">
          <FieldDescription>
            {t('buildPack.autoDetectDescription')}
          </FieldDescription>
          <div className="flex flex-wrap gap-2">
            {AUTO_DETECT_STACKS.map((stack) => (
              <span
                key={stack.icon}
                className="inline-flex items-center gap-1.5 rounded-md border border-input bg-muted/50 px-2 py-1 text-xs text-muted-foreground"
              >
                <BrandIcon name={stack.icon} className="size-3.5" />
                {stack.label}
              </span>
            ))}
          </div>
        </TabsContent>

        <TabsContent value="dockerfile" className="pt-2">
          <Field>
            <FieldLabel htmlFor="git-source-build-path">
              {t('buildPack.dockerfilePath')}
            </FieldLabel>
            <Input
              id="git-source-build-path"
              className="font-mono"
              placeholder="./Dockerfile"
              autoComplete="off"
              spellCheck={false}
              value={buildPath}
              onChange={(e) => {
                onBuildPath(e.target.value)
              }}
              disabled={disabled}
            />
          </Field>
        </TabsContent>

        <TabsContent value="static" className="pt-2">
          <FieldDescription>
            {t('buildPack.staticDescription')}
          </FieldDescription>
        </TabsContent>
      </Tabs>
    </Field>
  )
}
