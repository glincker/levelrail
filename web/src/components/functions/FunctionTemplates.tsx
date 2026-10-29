import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useBrand } from '@/hooks/useBrand'
import { useFunctionRuntimes } from '../../queries/functions'

export function CopyButton({ text }: Readonly<{ text: string }>) {
  const { t } = useTranslation('functions')
  const [copied, setCopied] = useState(false)
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={() => {
        void navigator.clipboard.writeText(text).then(() => {
          setCopied(true)
        })
      }}
    >
      {copied ? (
        <CheckGlassIcon size="1.2em" />
      ) : (
        <CopyGlassIcon size="1.2em" />
      )}
      {copied ? t('templates.copied') : t('templates.copy')}
    </Button>
  )
}

export function FunctionTemplates() {
  const { t } = useTranslation('functions')
  const brand = useBrand()
  const { data, error } = useFunctionRuntimes()
  if (error) {
    return (
      <p className="text-sm text-destructive">
        {t('templates.error', { message: error.message })}
      </p>
    )
  }
  if (!data || data.runtimes.length === 0) return null
  return (
    <section className="space-y-3" aria-labelledby="function-templates">
      <div>
        <h2 id="function-templates" className="text-base font-semibold">
          {t('templates.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('templates.description')}
        </p>
      </div>
      <Tabs defaultValue={data.runtimes[0]?.id}>
        <TabsList>
          {data.runtimes.map((r) => (
            <TabsTrigger key={r.id} value={r.id}>
              {r.name}
            </TabsTrigger>
          ))}
        </TabsList>
        {data.runtimes.map((r) => (
          <TabsContent key={r.id} value={r.id} className="space-y-3">
            {r.files.map((f) => (
              <div key={f.path} className="rounded-lg border border-input">
                <div className="flex items-center justify-between border-b border-input px-3 py-1.5">
                  <code className="text-xs">{f.path}</code>
                  <CopyButton text={f.content} />
                </div>
                <pre className="overflow-x-auto p-3 text-xs">
                  <code>{f.content}</code>
                </pre>
              </div>
            ))}
            <p className="text-xs text-muted-foreground">
              {t('templates.cliLabel')}:{' '}
              <code>
                {brand.BinaryName}-cli functions init {r.id}
              </code>
            </p>
          </TabsContent>
        ))}
      </Tabs>
    </section>
  )
}
