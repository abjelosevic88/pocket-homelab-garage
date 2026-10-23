import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, upload, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { VehicleMember } from '../../lib/types'
import { Card, Confirm, Field, useToast } from '../../components/ui'
import { Select } from '../../components/Select'
import { useVehicle } from './ctx'
import { Icon } from '../../components/icons'

export default function VehicleSettings() {
  const { vehicle: v, reload, overview } = useVehicle()
  const { users, user, settings, reloadVehicles } = useApp()
  const { toast } = useToast()
  const navigate = useNavigate()
  const [f, setF] = useState({ firefly_tag: v.firefly_tag, dawarich: v.dawarich })
  const [members, setMembers] = useState<VehicleMember[]>(v.members || [])
  const [del, setDel] = useState(false)
  const [busy, setBusy] = useState('')
  const isOwner = v.role === 'owner' && v.owner_id === user?.id
  const save = async () => { setBusy('save'); try { await api.put(`${V1}/vehicles/${v.id}`, f); toast('Saved'); reload() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') } }
  const saveMembers = async () => { setBusy('members'); try { await api.put(`${V1}/vehicles/${v.id}/members`, { members }); toast('Sharing saved'); reload() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') } }
  const img = async (file: File) => { try { await upload(`${V1}/vehicles/${v.id}/image`, 'file', file); toast('Photo saved'); reload(); reloadVehicles() } catch (e) { toast((e as Error).message, 'err') } }
  const others = users.filter(u => u.id !== v.owner_id)
  return (
    <div className="two-col">
      <div className="grid" style={{ gap: 16 }}>
        <Card title={<><Icon name="tag" size={16} />Tag</>}>
          <p className="muted small">Added to every transaction Garage books in Pocket Money for this vehicle (for example <code>skoda</code>), so Money can show what the car costs.{!isOwner && ' Only the owner changes it.'}</p>
          <div className="row">
            <input value={f.firefly_tag} onChange={e => setF({ ...f, firefly_tag: e.target.value })} placeholder="skoda" disabled={!isOwner} style={{ maxWidth: 240 }} />
            {isOwner && <button className="btn primary sm" onClick={save} disabled={busy === 'save'}>Save</button>}
          </div>
        </Card>
        <Card title={<><Icon name="route" size={16} />Location history</>} actions={<span className={`badge ${overview.integrations.dawarich ? 'cat' : ''}`}>{overview.integrations.dawarich ? `on · ${overview.location?.name || 'Dawarich'}` : settings?.location_works ? 'off' : 'not configured'}</span>}>
          <p className="muted small">Driving kilometres from your location history ({overview.location?.name || 'Dawarich'}, chosen under Settings → Connected apps) are added on top of your last odometer reading, so reminders by distance stay accurate between fill-ups. Switch it on for the vehicle you drive with the phone that records.</p>
          <label className="row"><input type="checkbox" checked={f.dawarich} onChange={e => setF({ ...f, dawarich: e.target.checked })} style={{ width: 'auto' }} disabled={!isOwner || !settings?.location_works} /> This vehicle drives the recorded kilometres{!isOwner && <span className="muted small"> (the owner's location history; only the owner switches it)</span>}</label>
          {isOwner && <div className="mt-sm"><button className="btn primary sm" onClick={save} disabled={busy === 'save'}>Save</button></div>}
        </Card>
        <Card title="Photo">
          <div className="row">{v.has_image && <img src={`${V1}/vehicles/${v.id}/image?v=${v.updated_at}`} alt="" style={{ width: 120, height: 80, objectFit: 'cover', borderRadius: 8 }} />}<label className="btn sm">Upload…<input type="file" accept="image/*" hidden onChange={e => e.target.files?.[0] && img(e.target.files[0])} /></label>{v.has_image && <button className="btn ghost sm" onClick={async () => { await api.del(`${V1}/vehicles/${v.id}/image`); reload(); reloadVehicles() }}>Remove</button>}</div>
        </Card>
      </div>
      <div className="grid" style={{ gap: 16 }}>
        <Card title="Sharing">
          <p className="muted small">Owner: <b>{v.members?.find(m => m.role === 'owner')?.name || '—'}</b>. Editors log and edit; viewers only read.</p>
          {others.length === 0 ? <p className="muted small">Add people under Settings → Users first.</p> : others.map(u => {
            const m = members.find(x => x.user_id === u.id)
            return <div key={u.id} className="row mb-sm" style={{ justifyContent: 'space-between' }}><span>{u.name || u.email}</span><Select value={m?.role || ''} onChange={r => setMembers(ms => r ? [...ms.filter(x => x.user_id !== u.id), { user_id: u.id, name: u.name, email: u.email, role: r as 'editor' }] : ms.filter(x => x.user_id !== u.id))} options={[{ value: 'editor', label: 'Editor' }, { value: 'viewer', label: 'Viewer' }]} placeholder="No access" clearLabel="No access" className="sm" /></div>
          })}
          {isOwner && others.length > 0 && <button className="btn primary sm" onClick={saveMembers} disabled={busy === 'members'}>Save sharing</button>}
        </Card>
        <Card title="Export">
          <div className="row" style={{ flexWrap: 'wrap' }}>
            <a className="btn sm" href={`${V1}/vehicles/${v.id}/export/vehicle.json`}>JSON backup</a>
            <a className="btn sm" href={`${V1}/vehicles/${v.id}/export/records.csv`}>Records CSV</a>
            <a className="btn sm" href={`${V1}/vehicles/${v.id}/export/fuel.csv`}>Fuel CSV</a>
            <a className="btn sm" href={`${V1}/vehicles/${v.id}/export/odometer.csv`}>Odometer CSV</a>
            <a className="btn sm" href={`${V1}/vehicles/${v.id}/export/calendar.ics`}>Calendar .ics</a>
          </div>
          <p className="muted small mt-sm">The JSON backup imports again under Settings → Import. Subscribe a phone to <code>{settings?.base_url || ''}/cal/{v.id}.ics?token=…</code> with an API token for live reminders.</p>
        </Card>
        {isOwner && <Card title="Danger zone"><button className="btn danger" onClick={() => setDel(true)}>Delete vehicle</button><p className="muted small mt-sm">Removes every fill-up, record, reminder, plan, note and document. Archive it instead if you sold it.</p></Card>}
      </div>
      {del && <Confirm title={`Delete ${v.display_name}?`} message="Everything about this vehicle is deleted. This cannot be undone." onCancel={() => setDel(false)} onConfirm={async () => { await api.del(`${V1}/vehicles/${v.id}`); reloadVehicles(); navigate('/vehicles') }} />}
    </div>
  )
}
