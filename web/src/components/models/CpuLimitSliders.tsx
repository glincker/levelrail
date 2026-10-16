import { useTranslation } from 'react-i18next'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { formatCpus, formatMemoryMiB } from '../../lib/modelsCpu'
import type { CpuBounds } from '../../types/modelsCpu'

const CPU_STEP_MILLI = 250
const MEMORY_STEP_MIB = 256
const MIN_MEMORY_MIB = 512

const RANGE_CLASS =
  'h-2 w-full cursor-pointer accent-primary focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none'

// Hard caps the engine container is created with. Ceilings come from the
// server, so the sliders never offer a value the API would refuse.
export function CpuLimitSliders({
  bounds,
  cpuMilli,
  memoryMiB,
  onCpu,
  onMemory,
}: {
  bounds: CpuBounds
  cpuMilli: number
  memoryMiB: number
  onCpu: (milli: number) => void
  onMemory: (mib: number) => void
}) {
  const { t } = useTranslation('models')
  const cpuMax = Math.max(bounds.max_cpu_milli, CPU_STEP_MILLI)
  const memMax = Math.max(bounds.max_memory_mib, MIN_MEMORY_MIB)
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <Field>
        <FieldLabel htmlFor="cpu-limit">
          {t('cpu.limits.cpu', { value: formatCpus(cpuMilli) })}
        </FieldLabel>
        <input
          id="cpu-limit"
          type="range"
          min={CPU_STEP_MILLI}
          max={cpuMax}
          step={CPU_STEP_MILLI}
          value={Math.min(cpuMilli, cpuMax)}
          onChange={(e) => {
            onCpu(Number(e.target.value))
          }}
          className={RANGE_CLASS}
        />
        <FieldHint>{t('cpu.limits.cpuHint')}</FieldHint>
      </Field>
      <Field>
        <FieldLabel htmlFor="memory-limit">
          {t('cpu.limits.memory', { value: formatMemoryMiB(memoryMiB) })}
        </FieldLabel>
        <input
          id="memory-limit"
          type="range"
          min={MIN_MEMORY_MIB}
          max={memMax}
          step={MEMORY_STEP_MIB}
          value={Math.min(memoryMiB, memMax)}
          onChange={(e) => {
            onMemory(Number(e.target.value))
          }}
          className={RANGE_CLASS}
        />
        <FieldHint>
          {t('cpu.limits.memoryHint', {
            floor: formatMemoryMiB(bounds.free_ram_floor_mib),
          })}
        </FieldHint>
      </Field>
    </div>
  )
}
