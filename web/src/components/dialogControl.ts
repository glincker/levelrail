/** Optional external control for a dialog that normally owns its own trigger. */
export interface DialogControl {
  open?: boolean
  onOpenChange?: (open: boolean) => void
  hideTrigger?: boolean
}
