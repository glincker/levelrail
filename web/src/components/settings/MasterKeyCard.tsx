import { KeyIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { HelpLink } from '@/components/HelpLink'
import { RotateMasterKeyDialog } from '@/components/RotateMasterKeyDialog'

// Only rendered when status.secrets_configured (master key rotation
// requires one already loaded, same 501-if-not-configured shape
// useRotateMasterKey's own doc comment establishes): showing the
// rotate action when there's no master key to rotate would just be a
// button that always fails, the same reasoning DiskUsageCard's own
// early return applies to a different missing-precondition case.
export function MasterKeyCard() {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <KeyIcon className="size-4" />
            </div>
            <div>
              <CardTitle className="flex items-center gap-1.5">
                Master key
                <HelpLink
                  path="/master-key-rotation"
                  label="Master key rotation guide"
                />
              </CardTitle>
              <CardDescription>
                Rotate the envelope-encryption key every stored secret depends
                on.
              </CardDescription>
            </div>
          </div>
          <RotateMasterKeyDialog />
        </div>
      </CardHeader>
    </Card>
  )
}
