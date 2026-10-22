import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

const NODES = [
  { x: 350, y: 100 },
  { x: 410, y: 150 },
  { x: 330, y: 185 },
]

export default function NodesDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('nodes')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '120 70 330 150'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4" className="stroke-(--iso-l)">
        <path d="M196 138L326 104" />
        <path d="M196 146L384 148" />
        <path d="M190 152L304 182" />
      </g>
      <g strokeWidth="2" className="stroke-(--iso-l)">
        <path d="M350 106L410 156" />
        <path d="M410 156L330 191" />
        <path d="M330 191L350 106" />
      </g>
      <Block x={170} y={140} a={40} h={12} p={BLUE} />
      <Block x={170} y={124} a={40} h={12} p={BLUE} />
      <circle cx={140} cy={136} r="1.8" className="fill-card" />
      <circle cx={140} cy={152} r="1.8" className="fill-card" />
      {NODES.map((n) => (
        <Block key={`${n.x}-${n.y}`} x={n.x} y={n.y} a={26} h={12} p={GREY} />
      ))}
      <Block x={368} y={148} a={8} h={8} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M170 92V101" />
            <path d="M350 76V86" />
            <path d="M368 160V230" strokeDasharray="2 3" />
          </g>
          <rect
            x="268"
            y="230"
            width="200"
            height="28"
            rx="11"
            className="fill-card stroke-border"
          />
          <g className={LABEL}>
            <text x="170" y="88" textAnchor="middle">
              {t('banner.diagram.controlPlane')}
            </text>
            <text x="350" y="72" textAnchor="middle">
              {t('banner.diagram.nodes')}
            </text>
            <text
              x="368"
              y="249"
              textAnchor="middle"
              className="fill-foreground"
            >
              {t('banner.diagram.mesh')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
