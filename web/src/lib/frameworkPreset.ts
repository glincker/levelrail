import type { FrameworkPreset } from '../queries/frameworks'

export function presetDiffers(
  preset: FrameworkPreset,
  port: string,
  healthPath: string,
): boolean {
  if (preset.build_type === 'static') return false
  return (
    String(preset.port) !== port.trim() ||
    preset.health_path !== healthPath.trim()
  )
}
