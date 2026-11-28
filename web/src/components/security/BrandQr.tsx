import { useMemo } from 'react'
import {
  FINDER,
  MODULE_FILL,
  QR_QUIET,
  buildQrModel,
  dataPath,
  finderOrigins,
} from './qrModel'

/** Brand colored QR with rounded modules and ring-and-dot finder eyes, always on a light tile. */
export default function BrandQr({
  value,
  label,
  size,
}: {
  value: string
  label: string
  size: number
}) {
  const model = useMemo(() => buildQrModel(value), [value])
  const d = useMemo(() => dataPath(model), [model])
  const full = model.size + QR_QUIET * 2
  return (
    <svg
      role="img"
      aria-label={label}
      width={size}
      height={size}
      viewBox={`0 0 ${full} ${full}`}
      className="block"
      shapeRendering="geometricPrecision"
    >
      <rect width={full} height={full} rx={1.5} className="fill-white" />
      <g transform={`translate(${QR_QUIET} ${QR_QUIET})`}>
        <path d={d} className={MODULE_FILL} data-qr="data" />
        {finderOrigins(model.size).map(([fx, fy]) => (
          <g key={`${fx}-${fy}`} data-qr="finder">
            <rect
              x={fx}
              y={fy}
              width={FINDER}
              height={FINDER}
              rx={2}
              className={MODULE_FILL}
            />
            <rect
              x={fx + 1}
              y={fy + 1}
              width={FINDER - 2}
              height={FINDER - 2}
              rx={1.2}
              className="fill-white"
            />
            <rect
              x={fx + 2}
              y={fy + 2}
              width={FINDER - 4}
              height={FINDER - 4}
              rx={1}
              className={MODULE_FILL}
            />
          </g>
        ))}
      </g>
    </svg>
  )
}
