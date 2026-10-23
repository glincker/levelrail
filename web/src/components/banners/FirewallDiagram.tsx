import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function FirewallDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('firewall')
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
        <path d="M146 150L228 122" />
        <path d="M272 120L352 130" />
        <path d="M146 178L226 164" />
      </g>
      <g className="stroke-destructive" strokeWidth="2" strokeLinecap="round">
        <path d="M230 158L240 168" />
        <path d="M240 158L230 168" />
      </g>
      <Block x={120} y={168} a={24} h={10} p={GREY} />
      <Block x={390} y={142} a={40} h={12} p={BLUE} />
      <Block x={390} y={126} a={40} h={12} p={BLUE} />
      <Block x={250} y={140} a={16} h={40} p={YELLOW} />
      <Block x={178} y={138} a={7} h={7} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M390 98V90" />
          </g>
          <g className={LABEL}>
            <text x="92" y="116">
              {t('banner.diagram.allowed')}
            </text>
            <text x="120" y="215" textAnchor="middle">
              {t('banner.diagram.blocked')}
            </text>
            <text x="390" y="82" textAnchor="middle">
              {t('banner.diagram.server')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
