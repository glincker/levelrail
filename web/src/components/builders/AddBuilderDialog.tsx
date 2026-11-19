import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { InfoTip } from '../kit/InfoTip'
import { useCreateNodeJoinToken } from '../../queries/nodes'
import { buildBuilderJoinCommand } from '../../lib/builderJoin'

// Minted on open and kept in state only: the server never returns the token again.
function TokenBody({ onClose }: Readonly<{ onClose: () => void }>) {
  const { t } = useTranslation('nodes')
  const mint = useCreateNodeJoinToken()
  const [copied, setCopied] = useState(false)
  const command = mint.data
    ? buildBuilderJoinCommand(window.location.origin, mint.data)
    : ''

  const copy = () => {
    void navigator.clipboard.writeText(command).then(() => {
      setCopied(true)
    })
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          {t('builders.add.title')}
          <InfoTip label={t('builders.add.tipLabel')}>
            {t('builders.add.tip')}
          </InfoTip>
        </DialogTitle>
        <DialogDescription>{t('builders.add.description')}</DialogDescription>
      </DialogHeader>
      <Tabs defaultValue="macos">
        <TabsList>
          <TabsTrigger value="macos">{t('builders.add.macos')}</TabsTrigger>
          <TabsTrigger value="linux">{t('builders.add.linux')}</TabsTrigger>
          <TabsTrigger value="windows" disabled>
            {t('builders.add.windows')}
          </TabsTrigger>
        </TabsList>
        {(['macos', 'linux'] as const).map((os) => (
          <TabsContent key={os} value={os} className="space-y-2">
            {mint.data ? (
              <div className="flex items-start gap-2 rounded-lg border border-input bg-muted/50 p-2">
                <code
                  aria-label={t('builders.add.commandLabel')}
                  className="min-w-0 flex-1 overflow-x-auto text-xs break-all whitespace-pre-wrap"
                >
                  {command}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={copy}
                >
                  {copied ? (
                    <CheckGlassIcon size="1.4em" />
                  ) : (
                    <CopyGlassIcon size="1.4em" />
                  )}
                  {copied ? t('builders.add.copied') : t('builders.add.copy')}
                </Button>
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                {mint.isPending
                  ? t('builders.add.creating')
                  : t('builders.add.notCreated')}
              </p>
            )}
            <p className="text-xs text-muted-foreground">
              {t(`builders.add.${os}Hint`)}
            </p>
          </TabsContent>
        ))}
      </Tabs>
      {mint.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{mint.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <DialogFooter>
        {mint.data ? (
          <span className="mr-auto text-xs text-muted-foreground">
            {t('builders.add.once')}
          </span>
        ) : (
          <Button
            variant="outline"
            disabled={mint.isPending}
            onClick={() => {
              mint.mutate({ builder: true })
            }}
          >
            {t('builders.add.create')}
          </Button>
        )}
        <Button onClick={onClose}>{t('builders.add.done')}</Button>
      </DialogFooter>
    </>
  )
}

export default function AddBuilderDialog({
  open,
  onOpenChange,
}: Readonly<{ open: boolean; onOpenChange: (open: boolean) => void }>) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <TokenBody
          key={open ? 'open' : 'closed'}
          onClose={() => {
            onOpenChange(false)
          }}
        />
      </DialogContent>
    </Dialog>
  )
}
