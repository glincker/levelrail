import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function CdnDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('cdn')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '70 55 350 150'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4" className="stroke-(--iso-l)">
        <path d="M226 118L146 148" />
        <path d="M260 138V164" />
        <path d="M294 118L374 148" />
      </g>
      <Block x={130} y={155} a={24} h={10} p={GREY} />
      <Block x={260} y={178} a={24} h={10} p={GREY} />
      <Block x={390} y={155} a={24} h={10} p={GREY} />
      <Block x={260} y={100} a={44} h={12} p={BLUE} />
      <Block x={260} y={84} a={44} h={12} p={BLUE} />
      <Block x={260} y={150} a={7} h={7} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M306 84H336" />
            <path d="M260 200V232" strokeDasharray="2 3" />
          </g>
          <rect
            x="210"
            y="232"
            width="100"
            height="28"
            rx="11"
            className="fill-card stroke-border"
          />
          <g className={LABEL}>
            <text x="340" y="89">
              {t('banner.diagram.origin')}
            </text>
            <text
              x="260"
              y="251"
              textAnchor="middle"
              className="fill-foreground"
            >
              {t('banner.diagram.edge')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
