import { SkeletonLine } from '@/components/kit'
import { useModelFit } from '../../queries/modelFit'
import { FitNodeList } from './ModelFitView'

// Where the model would fit, judged against each node's free VRAM. The
// node it runs on is marked current.
export function ModelFitCard({ modelName }: { modelName: string }) {
  const { data, isPending, error } = useModelFit(modelName)
  return (
    <section aria-label="VRAM fit" className="space-y-2">
      <h3 className="text-sm font-semibold">VRAM fit</h3>
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error.message}
        </p>
      ) : isPending ? (
        <SkeletonLine />
      ) : (
        <FitNodeList report={data} highlight={(n) => n.current} />
      )}
    </section>
  )
}
