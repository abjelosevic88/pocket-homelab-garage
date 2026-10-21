import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import type { Reminder } from '../../lib/types'
import { fmtDate } from '../../lib/format'
import { fmtOdo } from '../../lib/units'
import { Card, Confirm, Empty, Loading, Segmented, useAsync } from '../../components/ui'
import { ReminderModal } from '../../components/forms'
import { ReminderRow } from '../Reminders'
import { useVehicle } from './ctx'
import { Icon } from '../../components/icons'

export default function RemindersTab() {
  const { vehicle: v, reload: reloadOv, canEdit, overview } = useVehicle()
  const [status, setStatus] = useState<'active' | 'done'>('active')
  const { data, loading, reload } = useAsync(() => api.get<Reminder[]>(`${V1}/vehicles/${v.id}/reminders?status=${status}`), [v.id, status])
  const [creating, setCreating] = useState(false)
  const [del, setDel] = useState<Reminder | null>(null)
  const changed = () => { reload(); reloadOv() }
  const base = overview.integrations.notify
  return (
    <>
      <div className="toolbar">
        <Segmented value={status} onChange={setStatus} options={[{ value: 'active', label: 'Active' }, { value: 'done', label: 'Done' }]} size="sm" />
        <span className="spacer" />
        <a className="btn sm" href={`${V1}/vehicles/${v.id}/export/calendar.ics`} title="Download the reminders as a calendar"><Icon name="calendar" size={14} />.ics</a>
        {canEdit && <button className="btn primary sm" onClick={() => setCreating(true)}>+ Reminder</button>}
      </div>
      <Card flush>
        <div style={{ padding: '0 16px' }}>
          {loading && !data ? <Loading /> : !data || data.length === 0 ? <Empty title={status === 'active' ? 'No reminders' : 'Nothing done yet'}>{status === 'active' && 'Oil changes, registration, insurance, tyre swaps — by date, by odometer, or whichever comes first. Recurring ones roll forward when you mark them done.'}</Empty>
            : data.map(r => status === 'active' ? <ReminderRow key={r.id} r={r} vehicle={v} onChanged={changed} canEdit={canEdit} /> : (
              <div key={r.id} className="rem-row"><span className="ico"><Icon name="check" size={16} /></span><span><div className="t">{r.title}</div><div className="s">done {r.last_done_date ? fmtDate(r.last_done_date) : ''}{r.last_done_odo ? ` at ${fmtOdo(r.last_done_odo, v.odometer_unit)}` : ''}</div></span>{canEdit && <span className="acts"><button className="btn ghost sm danger-text" onClick={() => setDel(r)}>Delete</button></span>}</div>
            ))}
        </div>
      </Card>
      <p className="muted small mt">{base ? `Notifications are on: you get a push when a reminder becomes due soon, due or overdue.` : 'Set a notification URL under Settings → Integrations to get a push when something comes due.'} Subscribe your phone's calendar to <code>/cal/{v.id}.ics?token=…</code> with an API token.</p>
      {creating && <ReminderModal vehicle={v} onClose={() => setCreating(false)} onSaved={() => { setCreating(false); changed() }} />}
      {del && <Confirm title="Delete this reminder?" message="It is gone for good; the work it logged stays." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/vehicles/${v.id}/reminders/${del.id}`); setDel(null); changed() }} />}
    </>
  )
}
