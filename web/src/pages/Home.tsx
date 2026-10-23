import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { HomeData, Vehicle } from '../lib/types'
import { KIND_LABEL } from '../lib/types'
import { fmtDate } from '../lib/format'
import { fmtEcon, fmtOdo } from '../lib/units'
import { Card, Empty, Loading, PageHeader, useAsync } from '../components/ui'
import { Money } from '../components/Money'
import { VehicleCard } from './Vehicles'
import { ReminderRow } from './Reminders'
import { FuelModal, RecordModal, VehicleModal } from '../components/forms'
import { Icon, LOG_KIND_ICON } from '../components/icons'
import { useOnChange } from '../components/Layout'

export default function Home() {
  const { user, settings, reloadVehicles } = useApp()
  const navigate = useNavigate()
  const { data, error, loading, reload } = useAsync(() => api.get<HomeData>(`${V1}/home`), [])
  const [modal, setModal] = useState<'fuel' | 'record' | 'vehicle' | null>(null)
  useOnChange(() => { reload() })
  if (loading && !data) return <Loading />
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return null
  const { stats, vehicles, reminders, recent } = data
  const hour = new Date().getHours()
  const greet = hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening'
  const first = (user?.name || '').split(' ')[0] || 'there'
  const main: Vehicle | undefined = vehicles.find(v => v.status === 'active') || vehicles[0]
  const s = main?.summary
  const u = settings!
  const heroStyle = main?.has_image ? { backgroundImage: `url(${V1}/vehicles/${main.id}/image?v=${main.updated_at})` } : { background: `linear-gradient(135deg, ${main?.color || '#c2410c'} 0%, #1f2937 100%)` }
  return (
    <>
      <PageHeader title={`${greet}, ${first}`} sub={stats.due > 0 ? `${stats.due} reminder${stats.due === 1 ? '' : 's'} need attention.` : stats.soon > 0 ? `${stats.soon} coming up soon.` : vehicles.length ? 'All quiet in the garage.' : 'Add your first vehicle.'}
        actions={<>{main && <button className="btn primary" onClick={() => setModal('fuel')}><Icon name="fuel" size={15} />Fill-up</button>}{main && <button className="btn" onClick={() => setModal('record')}><Icon name="wrench" size={15} />Log work</button>}<button className="btn" onClick={() => setModal('vehicle')}><Icon name="plus" size={15} />Vehicle</button></>} />

      {main && s ? (
        <div className={`hero garage ${main.has_image ? 'has-cover' : ''}`} style={heroStyle} onClick={() => navigate(`/vehicles/${main.id}`)}>
          <div className="hero-body">
            <div>
              <div className="row" style={{ gap: 8, marginBottom: 6 }}>{main.plate && <span className="status-pill">{main.plate}</span>}{s.reminders_due > 0 && <span className="status-pill"><Icon name="alert" size={12} strokeWidth={2.2} />{s.reminders_due} due</span>}{s.estimate_km > 0 && <span className="status-pill" title="Kilometres driven (location history) since your last confirmed reading">≈ +{Math.round(s.estimate_km)} {main.odometer_unit} recorded</span>}</div>
              <h1>{main.display_name}</h1>
              <div className="odo-big">{fmtOdo(s.odometer_estimate, main.odometer_unit, false)}<small>{main.odometer_unit}{s.estimate_km > 0 ? ' est.' : ''}</small></div>
              <div className="meta-line"><span>{s.economy ? fmtEcon(s.economy_recent || s.economy, u, main.odometer_unit) : 'no economy yet'}</span><span><Money minor={s.cost_year} code={main.currency} compact /> this year</span>{s.last_fuel && <span>last fill {fmtDate(s.last_fuel.date, false)}</span>}</div>
            </div>
            <div className="row" onClick={e => e.stopPropagation()}>
              <Link className="btn ghost" to={`/vehicles/${main.id}/fuel`}>Fuel</Link>
              <Link className="btn ghost" to={`/vehicles/${main.id}/service`}>Service</Link>
              <Link className="btn ghost" to={`/vehicles/${main.id}/reminders`}>Reminders</Link>
            </div>
          </div>
        </div>
      ) : (
        <Card className="mb"><Empty icon="car" title="No vehicles yet">Add one, or bring everything over from LubeLogger under Settings → Import.</Empty></Card>
      )}

      <div className="stats">
        <div className="stat"><div className="n"><Money minor={stats.cost_month} code={data.currency} compact /></div><div className="l">this month</div></div>
        <div className="stat"><div className="n"><Money minor={stats.cost_year} code={data.currency} compact /></div><div className="l">this year, all vehicles</div></div>
        <Link className="stat" to="/reminders"><div className="n">{stats.due}<span className="muted" style={{ fontSize: 14 }}> due</span></div><div className="l">{stats.soon} coming up</div></Link>
        <Link className="stat" to="/vehicles"><div className="n">{stats.active}</div><div className="l">active vehicle{stats.active === 1 ? '' : 's'}</div></Link>
      </div>

      <div className="home-grid">
        <div>
          {reminders.length > 0 && <Card title="Needs attention" actions={<Link className="btn ghost sm" to="/reminders">All reminders</Link>} className="mb">
            {reminders.slice(0, 6).map(r => <ReminderRow key={r.id} r={r} showVehicle onChanged={() => { reload(); reloadVehicles() }} />)}
          </Card>}
          {vehicles.length > 1 && <><h2 className="mb-sm">Vehicles</h2><div className="vehicle-grid mb">{vehicles.map(v => <VehicleCard key={v.id} v={v} />)}</div></>}
        </div>
        <Card title="Latest entries">
          {recent.length === 0 ? <p className="muted">Fill-ups and work you log show up here.</p> : (
            <div className="inline-list">{recent.map(e => <Link key={`${e.kind}-${e.id}`} className="li" to={`/vehicles/${e.vehicle_id}/${e.kind === 'fuel' ? 'fuel' : 'service'}`} style={{ color: 'inherit' }}>
              <Icon name={LOG_KIND_ICON[e.kind] || 'wrench'} size={15} title={KIND_LABEL[e.kind]} /><span className="grow"><b>{e.title}</b>{vehicles.length > 1 && <span className="muted small"> · {e.vehicle_name}</span>}</span><span className="muted small">{fmtDate(e.date, false)}</span>{e.cost > 0 && <Money minor={e.cost} code={e.currency} />}
            </Link>)}</div>
          )}
        </Card>
      </div>

      {modal === 'fuel' && main && <FuelModal vehicle={main} onClose={() => setModal(null)} onSaved={() => { setModal(null); reload() }} />}
      {modal === 'record' && main && <RecordModal vehicle={main} onClose={() => setModal(null)} onSaved={() => { setModal(null); reload() }} />}
      {modal === 'vehicle' && <VehicleModal onClose={() => setModal(null)} onSaved={v => { setModal(null); reloadVehicles(); navigate(`/vehicles/${v.id}`) }} />}
    </>
  )
}
