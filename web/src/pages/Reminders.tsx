import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Reminder, Vehicle } from '../lib/types'
import { KIND_ICON, URGENCY_LABEL } from '../lib/types'
import { fmtDate } from '../lib/format'
import { Card, Empty, Loading, PageHeader, Segmented, useAsync, useToast } from '../components/ui'
import { DoneModal, ReminderModal } from '../components/forms'

export default function Reminders() {
  const [filter, setFilter] = useState<'attention' | 'all'>('attention')
  const { data, loading, error, reload } = useAsync(() => api.get<Reminder[]>(`${V1}/reminders`), [])
  const list = (data || []).filter(r => filter === 'all' || r.urgency !== 'ok')
  return (
    <>
      <PageHeader title="Reminders" sub="Across every vehicle, most urgent first" />
      <div className="toolbar"><Segmented value={filter} onChange={setFilter} options={[{ value: 'attention', label: 'Needs attention' }, { value: 'all', label: 'All active' }]} size="sm" /></div>
      {loading && !data ? <Loading /> : error ? <div className="callout danger">{error}</div> : list.length === 0 ? <Card><Empty title={filter === 'all' ? 'No reminders yet' : 'Nothing due'}>{filter === 'all' ? 'Add reminders on a vehicle\'s Reminders tab — oil changes, registration, tyres…' : 'Everything is scheduled comfortably ahead.'}</Empty></Card> : (
        <Card flush><div style={{ padding: '0 16px' }}>{list.map(r => <ReminderRow key={r.id} r={r} showVehicle onChanged={reload} />)}</div></Card>
      )}
    </>
  )
}

function describe(r: Reminder, unit: string): string {
  const parts: string[] = []
  if (r.days_left != null) parts.push(r.days_left < 0 ? `${-r.days_left} d overdue` : r.days_left === 0 ? 'today' : `in ${r.days_left} d (${fmtDate(r.due_date, false)})`)
  if (r.km_left != null) parts.push(r.km_left < 0 ? `${Math.round(-r.km_left).toLocaleString()} ${unit} over` : `in ${Math.round(r.km_left).toLocaleString()} ${unit}`)
  if ((r.interval_days || r.interval_km) && parts.length) parts.push('repeats')
  return parts.join(' · ')
}

export function ReminderRow({ r, showVehicle, vehicle, onChanged, canEdit = true }: { r: Reminder; showVehicle?: boolean; vehicle?: Vehicle; onChanged: () => void; canEdit?: boolean }) {
  const { vehicles } = useApp()
  const { toast } = useToast()
  const [done, setDone] = useState(false)
  const [edit, setEdit] = useState(false)
  const v = vehicle || vehicles.find(x => x.id === r.vehicle_id)
  const unit = v?.odometer_unit || 'km'
  const snooze = async () => { try { await api.post(`${V1}/vehicles/${r.vehicle_id}/reminders/${r.id}/snooze`, { days: 7 }); toast('Snoozed a week'); onChanged() } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="rem-row">
      <span className="ico">{KIND_ICON[r.kind] || '🔔'}</span>
      <span style={{ minWidth: 0 }}>
        <div className="t">{r.title} <span className={`urg ${r.urgency}`}>{URGENCY_LABEL[r.urgency]}</span></div>
        <div className="s">{showVehicle && r.vehicle_name && <Link to={`/vehicles/${r.vehicle_id}/reminders`}>{r.vehicle_name}</Link>}{showVehicle && r.vehicle_name && ' · '}{describe(r, unit)}{r.notes && <> · {r.notes}</>}</div>
      </span>
      {canEdit && <span className="acts">
        <button className="btn sm primary" onClick={() => setDone(true)}>Done</button>
        <button className="btn sm ghost" onClick={snooze} title="Push the due date a week">Snooze</button>
        <button className="btn sm ghost" onClick={() => setEdit(true)}>Edit</button>
      </span>}
      {done && v && <DoneModal vehicle={v} reminder={r} onClose={() => setDone(false)} onDone={() => { setDone(false); onChanged() }} />}
      {edit && v && <ReminderModal vehicle={v} reminder={r} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); onChanged() }} />}
    </div>
  )
}
