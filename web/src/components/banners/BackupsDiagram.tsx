import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

function DatabaseStack({ x, y }: { x: number; y: number }) {
  return (
    <g>
      {[0, 1, 2].map((i) => (
        <g key={i}>
          <Block x={x} y={y - i * 17} a={34} h={11} p={BLUE} />
          <circle
            cx={x - 24}
            cy={y - i * 17 + 3}
            r="1.8"
            className="fill-card"
          />
          <circle
            cx={x - 18}
            cy={y - i * 17 + 6}
            r="1.8"
            className="fill-card"
          />
        </g>
      ))}
    </g>
  )
}

export default function BackupsDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('backups')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '70 90 400 160'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4">
        <path d="M176 192L340 190" className="stroke-(--iso-l)" />
        <path d="M270 138V186" className="stroke-(--iso-accent-l)" />
      </g>
      <path
        d="M332 184L341 190L332 196"
        className="stroke-(--iso-l)"
        strokeWidth="1.5"
      />
      <DatabaseStack x={130} y={185} />
      <Block x={390} y={190} a={46} h={18} p={GREY} />
      <Block x={390} y={178} a={14} h={8} p={YELLOW} />
      <circle
        cx="270"
        cy="120"
        r="17"
        className="fill-card stroke-(--iso-r)"
        strokeWidth="1.5"
      />
      <path
        d="M270 109V120L277 125"
        className="stroke-(--iso-r)"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M130 128V118" />
            <path d="M390 160V138" />
            <path d="M270 102V94" />
          </g>
          <g className={LABEL} textAnchor="middle">
            <text x="130" y="110">
              {t('banner.diagram.database')}
            </text>
            <text x="390" y="130">
              {t('banner.diagram.storage')}
            </text>
            <text x="270" y="86">
              {t('banner.diagram.schedule')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
