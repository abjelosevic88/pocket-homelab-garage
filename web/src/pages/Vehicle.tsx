import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Overview } from '../lib/types'
import { VEHICLE_ICON } from '../lib/types'
import { fmtOdo } from '../lib/units'
import { Loading, PageHeader, Tabs } from '../components/ui'
import { FuelModal, RecordModal, VehicleModal } from '../components/forms'
import { VCtx } from './vehicle/ctx'
import OverviewTab from './vehicle/Overview'
import FuelTab from './vehicle/FuelTab'
import ServiceTab from './vehicle/Service'
import OdometerTab from './vehicle/Odometer'
import RemindersTab from './vehicle/RemindersTab'
import Planner from './vehicle/Planner'
import Notes from './vehicle/Notes'
import Documents from './vehicle/Documents'
import Reports from './vehicle/Reports'
import VehicleSettings from './vehicle/VehicleSettings'

type Tab = 'overview' | 'fuel' | 'service' | 'odometer' | 'reminders' | 'planner' | 'notes' | 'documents' | 'reports' | 'settings'
const TABS: { id: Tab; label: string }[] = [
  { id: 'overview', label: 'Overview' }, { id: 'fuel', label: 'Fuel' }, { id: 'service', label: 'Service & costs' }, { id: 'odometer', label: 'Odometer' }, { id: 'reminders', label: 'Reminders' },
  { id: 'planner', label: 'Planner' }, { id: 'notes', label: 'Notes' }, { id: 'documents', label: 'Documents' }, { id: 'reports', label: 'Reports' }, { id: 'settings', label: 'Settings' },
]

export default function VehiclePage() {
  const { id, tab = 'overview' } = useParams()
  const navigate = useNavigate()
  const { user, reloadVehicles } = useApp()
  const [ov, setOv] = useState<Overview | null>(null)
  const [err, setErr] = useState('')
  const [modal, setModal] = useState<'fuel' | 'record' | 'edit' | null>(null)
  const load = async () => {
    try {
      const o = await api.get<Overview>(`${V1}/vehicles/${id}/overview`)
      // never let a null list from the API blank the page
      for (const k of ['reminders', 'recent_records', 'recent_fuel', 'pinned_notes', 'expiring_documents', 'months', 'activity', 'trips'] as const) if (!o[k]) (o as unknown as Record<string, unknown>)[k] = []
      setOv(o); setErr('')
    } catch (e) { setErr((e as Error).message) }
  }
  useEffect(() => { setOv(null); load() }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  if (err) return <div className="callout danger">{err} — <Link to="/vehicles">back to vehicles</Link></div>
  if (!ov) return <Loading />
  const v = ov.vehicle
  const s = v.summary!
  const canEdit = v.role !== 'viewer' || user?.role === 'admin'
  const t = (TABS.find(x => x.id === tab) ? tab : 'overview') as Tab
  return (
    <VCtx.Provider value={{ vehicle: v, overview: ov, reload: load, canEdit }}>
      <PageHeader title={<span className="row" style={{ gap: 10 }}>{v.has_image ? <img src={`${V1}/vehicles/${v.id}/image?v=${v.updated_at}`} alt="" style={{ width: 40, height: 40, borderRadius: 10, objectFit: 'cover' }} /> : <span style={{ fontSize: 28 }}>{VEHICLE_ICON[v.kind] || '🚗'}</span>}{v.display_name}{v.status !== 'active' && <span className="badge">{v.status}</span>}</span>}
        sub={<>{[v.year && v.name ? `${v.year} ${v.make} ${v.model}` : '', v.trim, v.plate, fmtOdo(s.odometer_estimate, v.odometer_unit) + (s.estimate_km > 0 ? ' est.' : '')].filter(Boolean).join(' · ')}</>}
        actions={canEdit && <>{v.status === 'active' && <button className="btn" onClick={() => setModal('fuel')}>⛽ Fill-up</button>}<button className="btn" onClick={() => setModal('record')}>🔧 Log work</button><button className="btn ghost" onClick={() => setModal('edit')}>Edit</button></>} />
      <Tabs tabs={TABS} value={t} onChange={x => navigate(`/vehicles/${v.id}/${x === 'overview' ? '' : x}`)} />
      <div className="mt">
        {t === 'overview' && <OverviewTab />}
        {t === 'fuel' && <FuelTab />}
        {t === 'service' && <ServiceTab />}
        {t === 'odometer' && <OdometerTab />}
        {t === 'reminders' && <RemindersTab />}
        {t === 'planner' && <Planner />}
        {t === 'notes' && <Notes />}
        {t === 'documents' && <Documents />}
        {t === 'reports' && <Reports />}
        {t === 'settings' && <VehicleSettings />}
      </div>
      {modal === 'fuel' && <FuelModal vehicle={v} onClose={() => setModal(null)} onSaved={() => { setModal(null); load() }} />}
      {modal === 'record' && <RecordModal vehicle={v} onClose={() => setModal(null)} onSaved={() => { setModal(null); load() }} />}
      {modal === 'edit' && <VehicleModal vehicle={v} onClose={() => setModal(null)} onSaved={() => { setModal(null); load(); reloadVehicles() }} />}
    </VCtx.Provider>
  )
}
