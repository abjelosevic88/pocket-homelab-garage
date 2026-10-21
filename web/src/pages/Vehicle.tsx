import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Overview } from '../lib/types'
import { fmtOdo } from '../lib/units'
import { Loading, PageHeader, Tabs } from '../components/ui'
import { FuelModal, RecordModal, VehicleModal } from '../components/forms'
import { Icon } from '../components/icons'
import { VehicleIcon } from '../components/vehicle'
import { useOnChange } from '../components/Layout'
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
const TABS: { id: Tab; label: string; icon: string }[] = [
  { id: 'overview', label: 'Overview', icon: 'car' }, { id: 'fuel', label: 'Fuel', icon: 'fuel' }, { id: 'service', label: 'Service & costs', icon: 'wrench' }, { id: 'odometer', label: 'Odometer', icon: 'gauge' }, { id: 'reminders', label: 'Reminders', icon: 'bell' },
  { id: 'planner', label: 'Planner', icon: 'kanban' }, { id: 'notes', label: 'Notes', icon: 'note' }, { id: 'documents', label: 'Documents', icon: 'file' }, { id: 'reports', label: 'Reports', icon: 'chart' }, { id: 'settings', label: 'Settings', icon: 'settings' },
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
  useOnChange(() => { load() })
  if (err) return <div className="callout danger"><Icon name="alert" size={15} /><span>{err} — <Link to="/vehicles">back to vehicles</Link></span></div>
  if (!ov) return <Loading />
  const v = ov.vehicle
  const s = v.summary!
  const canEdit = v.role !== 'viewer'
  const t = (TABS.find(x => x.id === tab) ? tab : 'overview') as Tab
  return (
    <VCtx.Provider value={{ vehicle: v, overview: ov, reload: load, canEdit }}>
      <PageHeader title={<span className="veh-title"><VehicleIcon v={v} size={40} />{v.display_name}{v.status !== 'active' && <span className="badge">{v.status}</span>}</span>}
        sub={<>{[v.year && v.name ? `${v.year} ${v.make} ${v.model}` : '', v.trim, v.plate, fmtOdo(s.odometer_estimate, v.odometer_unit) + (s.estimate_km > 0 ? ' est.' : '')].filter(Boolean).join(' · ')}</>}
        actions={canEdit && <>{v.status === 'active' && <button className="btn primary" onClick={() => setModal('fuel')}><Icon name="fuel" size={15} />Fill-up</button>}<button className="btn" onClick={() => setModal('record')}><Icon name="wrench" size={15} />Log work</button><button className="btn ghost" onClick={() => setModal('edit')}><Icon name="edit" size={15} />Edit</button></>} />
      <div className="veh-tabs"><Tabs tabs={TABS} value={t} onChange={x => navigate(`/vehicles/${v.id}/${x === 'overview' ? '' : x}`)} /></div>
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
