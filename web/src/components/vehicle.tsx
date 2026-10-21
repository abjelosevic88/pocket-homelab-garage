import { V1 } from '../lib/api'
import type { Vehicle } from '../lib/types'
import { Icon, VEHICLE_KIND_ICON } from './icons'

/** The vehicle's photo, or its kind's icon on its colour. */
export function VehicleIcon({ v, size = 22 }: { v: Pick<Vehicle, 'id' | 'kind' | 'color' | 'has_image' | 'updated_at'>; size?: number }) {
  return (
    <span className="veh-ico" style={{ width: size, height: size, borderRadius: Math.round(size * 0.28), ['--c' as string]: v.color || 'var(--accent)' }} aria-hidden="true">
      {v.has_image ? <img src={`${V1}/vehicles/${v.id}/image?v=${v.updated_at}`} alt="" /> : <Icon name={VEHICLE_KIND_ICON[v.kind] || 'car'} size={Math.round(size * 0.62)} strokeWidth={2} />}
    </span>
  )
}
