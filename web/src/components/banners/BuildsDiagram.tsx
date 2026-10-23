import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function BuildsDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('buildCenter')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '30 100 460 150'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4" className="stroke-(--iso-l)">
        <path d="M116 160L208 155" />
        <path d="M312 150L410 146" />
        <path d="M260 194V212" />
      </g>
      <Block x={80} y={162} a={28} h={14} p={GREY} />
      <Block x={80} y={150} a={9} h={9} p={YELLOW} />
      <Block x={260} y={150} a={48} h={18} p={BLUE} />
      <Block x={440} y={146} a={26} h={26} p={YELLOW} />
      <Block x={260} y={228} a={24} h={12} p={GREY} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M80 126V110" />
            <path d="M260 122V108" />
            <path d="M440 126V110" />
            <path d="M260 250V262" />
          </g>
          <g className={LABEL} textAnchor="middle">
            <text x="80" y="100">
              {t('banner.diagram.source')}
            </text>
            <text x="260" y="100">
              {t('banner.diagram.build')}
            </text>
            <text x="440" y="100">
              {t('banner.diagram.image')}
            </text>
            <text x="260" y="282">
              {t('banner.diagram.cache')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
