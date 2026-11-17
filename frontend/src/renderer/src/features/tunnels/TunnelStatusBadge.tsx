import { CircleCheck, CircleMinus, CircleAlert, CircleDashed, Octagon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { TunnelStatus } from '../../../../shared/contracts/tunnels'
import { t } from '../../../../shared/i18n'

const appearances = {
  running: {
    icon: CircleCheck,
    className: 'bg-green-500/10 text-green-700 dark:text-green-400'
  },
  starting: {
    icon: CircleDashed,
    className: 'bg-blue-500/10 text-blue-700 dark:text-blue-400'
  },
  stopping: {
    icon: Octagon,
    className: 'bg-amber-500/10 text-amber-800 dark:text-amber-300'
  },
  stopped: {
    icon: CircleMinus,
    className: 'bg-muted text-muted-foreground'
  },
  failed: {
    icon: CircleAlert,
    className: 'bg-destructive/10 text-destructive'
  }
} satisfies Record<TunnelStatus['state'], { icon: typeof CircleCheck; className: string }>

export function TunnelStatusBadge({ state }: { state: TunnelStatus['state'] }) {
  useTranslation()
  const { icon: Icon, className } = appearances[state]
  return (
    <Badge variant="secondary" className={className}>
      <Icon aria-hidden="true" />
      {t(`hosts:tunnels.${state}`)}
    </Badge>
  )
}
