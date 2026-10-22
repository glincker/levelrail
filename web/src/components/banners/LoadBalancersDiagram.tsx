import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL } from './isoTokens'

export default function LoadBalancersDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('common')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '40 70 420 195'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4" className="stroke-(--iso-l)">
        <path d="M104 166L186 162" />
        <path d="M272 150L390 110" />
        <path d="M272 176L390 214" />
      </g>
      <Block x={70} y={160} a={28} h={14} p={GREY} />
      <rect
        x="61"
        y="155"
        width="18"
        height="11"
        rx="1.5"
        className="stroke-(--iso-sleep-r)"
        strokeWidth="1.2"
      />
      <path
        d="M61 158H79"
        className="stroke-(--iso-sleep-r)"
        strokeWidth="1.2"
      />
      <Block x={230} y={160} a={44} h={18} p={BLUE} />
      <Block x={420} y={105} a={28} h={16} p={BLUE} />
      <Block x={420} y={215} a={28} h={16} p={BLUE} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M70 134V122" />
            <path d="M230 132V120" />
            <path d="M420 84V76" />
            <path d="M420 250V258" />
          </g>
          <g className={LABEL} textAnchor="middle">
            <text x="70" y="114">
              {t('banners.loadBalancers.diagram.browser')}
            </text>
            <text x="230" y="112">
              {t('banners.loadBalancers.diagram.balancer')}
            </text>
            <text x="420" y="68">
              {t('banners.loadBalancers.diagram.server')}
            </text>
            <text x="420" y="276">
              {t('banners.loadBalancers.diagram.server')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
