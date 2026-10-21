import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Vehicle } from '../lib/types'
import { fmtEcon, fmtOdo } from '../lib/units'
import { Empty, Loading, PageHeader, Segmented, useAsync } from '../components/ui'
import { Money } from '../components/Money'
import { VehicleModal } from '../components/forms'
import { Icon, VEHICLE_KIND_ICON } from '../components/icons'

export default function Vehicles() {
  const [status, setStatus] = useState<'active' | 'sold' | 'archived' | ''>('active')
  const [creating, setCreating] = useState(false)
  const navigate = useNavigate()
  const { reloadVehicles } = useApp()
  const { data, loading, error } = useAsync(() => api.get<Vehicle[]>(`${V1}/vehicles${status ? `?status=${status}` : ''}`), [status])
  return (
    <>
      <PageHeader title="Vehicles" sub={data ? `${data.length} ${status || 'total'}` : ''} actions={<button className="btn primary" onClick={() => setCreating(true)}><Icon name="plus" size={15} />Add vehicle</button>} />
      <div className="toolbar"><Segmented value={status} onChange={setStatus} options={[{ value: 'active', label: 'Active' }, { value: 'sold', label: 'Sold' }, { value: 'archived', label: 'Archived' }, { value: '', label: 'All' }]} size="sm" /></div>
      {loading && !data ? <Loading /> : error ? <div className="callout danger">{error}</div> : !data || data.length === 0 ? (
        <Empty icon="car" title={status === 'active' ? 'No vehicles yet' : `Nothing ${status}`}>{status === 'active' && 'Add one, or import from LubeLogger under Settings → Import.'}</Empty>
      ) : <div className="vehicle-grid">{data.map(v => <VehicleCard key={v.id} v={v} />)}</div>}
      {creating && <VehicleModal onClose={() => setCreating(false)} onSaved={v => { setCreating(false); reloadVehicles(); navigate(`/vehicles/${v.id}`) }} />}
    </>
  )
}

export function VehicleCard({ v }: { v: Vehicle }) {
  const { settings } = useApp()
  const s = v.summary
  const style = { '--c': v.color || '#c2410c', backgroundImage: v.has_image ? `url(${V1}/vehicles/${v.id}/image?v=${v.updated_at})` : undefined } as React.CSSProperties
  return (
    <Link to={`/vehicles/${v.id}`} className="vehicle-card">
      <div className={`cover ${v.has_image ? 'has-img' : ''}`} style={style}>
        {!v.has_image && <Icon name={VEHICLE_KIND_ICON[v.kind] || 'car'} size={52} strokeWidth={1.5} />}
        {v.status !== 'active' ? <span className="badge">{v.status}</span> : s && s.reminders_due > 0 ? <span className="badge danger">{s.reminders_due} due</span> : s && s.reminders_soon > 0 ? <span className="badge warn">{s.reminders_soon} soon</span> : null}
        {v.plate && <span className="plate">{v.plate}</span>}
      </div>
      <div className="body">
        <h3>{v.display_name}</h3>
        {s && <>
          <div className="muted small">{fmtOdo(s.odometer_estimate, v.odometer_unit)}{s.estimate_km > 0 && ' est.'}{s.economy > 0 && settings && <> · {fmtEcon(s.economy_recent || s.economy, settings, v.odometer_unit)}</>}</div>
          <div className="facts"><span title="Fill-ups"><Icon name="fuel" size={13} />{s.fills}</span><span title="Records"><Icon name="wrench" size={13} />{s.records}</span>{s.reminders > 0 && <span title="Reminders"><Icon name="bell" size={13} />{s.reminders}</span>}<span className="spacer right"><Money minor={s.cost_year} code={v.currency} compact /> <span className="muted">this year</span></span></div>
        </>}
      </div>
    </Link>
  )
}
