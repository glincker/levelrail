import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

export function RemoveDialog({
  appName,
  open,
  pending,
  onOpenChange,
  onConfirm,
}: {
  appName: string
  open: boolean
  pending: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Remove load balancer?</DialogTitle>
          <DialogDescription>
            Domains for {appName} go back to a single upstream. Running replicas
            are not touched.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={pending}
            onClick={onConfirm}
          >
            Remove
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
