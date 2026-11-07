import { createFileRoute } from '@tanstack/react-router'
import { ImageScanningPage } from '../../components/security/ImageScanningPage'

export const Route = createFileRoute('/security/image-scanning')({
  component: ImageScanningPage,
})
