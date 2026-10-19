import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, upload, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { VehicleMember } from '../../lib/types'
import { fmtDateTime } from '../../lib/format'
import { Card, Confirm, Field, useToast } from '../../components/ui'
import { Select } from '../../components/Select'
import { useVehicle } from './ctx'

export default function VehicleSettings() {
  const { vehicle: v, reload, overview } = useVehicle()
  const { users, user, settings, reloadVehicles } = useApp()
  const { toast } = useToast()
  const navigate = useNavigate()
  const [f, setF] = useState({ firefly_tag: v.firefly_tag, firefly_account: v.firefly_account, dawarich: v.dawarich })
  const [tags, setTags] = useState<{ id: string; name: string }[]>([])
  const [accounts, setAccounts] = useState<{ id: string; name: string; currency: string }[]>([])
  const [members, setMembers] = useState<VehicleMember[]>(v.members || [])
  const [del, setDel] = useState(false)
  const [busy, setBusy] = useState('')
  const [sync, setSync] = useState<{ fetched: number; matched: number; created: number; unmatched: number; notes?: string[] } | null>(null)
  const [unmatched, setUnmatched] = useState<{ unmatched: { id: string; date: string; description: string; amount: number; currency: string; hint: string }[]; synced_at: string } | null>(null)
  const isOwner = v.role === 'owner' && v.owner_id === user?.id
  useEffect(() => {
    if (isOwner && settings?.firefly_configured) { api.get<typeof tags>(`${V1}/firefly/tags`).then(setTags).catch(() => {}); api.get<typeof accounts>(`${V1}/firefly/accounts`).then(setAccounts).catch(() => {}) }
    api.get<typeof unmatched>(`${V1}/vehicles/${v.id}/firefly/unmatched`).then(setUnmatched).catch(() => {})
  }, [isOwner, settings?.firefly_configured, v.id])
  const save = async () => { setBusy('save'); try { await api.put(`${V1}/vehicles/${v.id}`, f); toast('Saved'); reload() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') } }
  const runSync = async () => { setBusy('sync'); try { setSync(await api.post(`${V1}/vehicles/${v.id}/firefly/sync`)); setUnmatched(await api.get(`${V1}/vehicles/${v.id}/firefly/unmatched`)); reload() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') } }
  const saveMembers = async () => { setBusy('members'); try { await api.put(`${V1}/vehicles/${v.id}/members`, { members }); toast('Sharing saved'); reload() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') } }
  const img = async (file: File) => { try { await upload(`${V1}/vehicles/${v.id}/image`, 'file', file); toast('Photo saved'); reload(); reloadVehicles() } catch (e) { toast((e as Error).message, 'err') } }
  const others = users.filter(u => u.id !== v.owner_id)
  return (
    <div className="two-col">
      <div className="grid" style={{ gap: 16 }}>
        <Card title="💶 Firefly III" actions={<span className={`badge ${overview.integrations.firefly ? 'cat' : ''}`}>{overview.integrations.firefly ? 'linked' : settings?.firefly_configured ? 'no tag' : 'not configured'}</span>}>
          {!isOwner && <p className="muted small">Linked to the owner's Firefly; only the owner picks the tag and the account.</p>}
          <p className="muted small">Tag the car's withdrawals in Firefly with one tag. Matching rows (same date and amount) are linked to the fill-up or record you logged; anything else becomes an expense here, so the vehicle total is never short. “→ Firefly” on a row creates the withdrawal the other way.</p>
          <div className="grid" style={{ gap: 12 }}>
            {isOwner && <><Field label="Firefly tag">{tags.length > 0 ? <Select value={f.firefly_tag} onChange={t => setF({ ...f, firefly_tag: t })} options={tags.map(t => ({ value: t.name, label: t.name }))} clearLabel="No tag" onCreate={t => t} createLabel="Use tag" /> : <input value={f.firefly_tag} onChange={e => setF({ ...f, firefly_tag: e.target.value })} placeholder="skoda" />}</Field>
            <Field label="Pay from (for pushed withdrawals)" help="Overrides the default account in Settings → Integrations.">{accounts.length > 0 ? <Select value={f.firefly_account} onChange={a => setF({ ...f, firefly_account: a })} options={accounts.map(a => ({ value: a.name, label: a.name, hint: a.currency }))} clearLabel="Use the default" /> : <input value={f.firefly_account} onChange={e => setF({ ...f, firefly_account: e.target.value })} />}</Field></>}
            <div className="row">{isOwner && <button className="btn primary sm" onClick={save} disabled={busy === 'save'}>Save</button>}{overview.integrations.firefly && <button className="btn sm" onClick={runSync} disabled={busy === 'sync'}>{busy === 'sync' ? 'Syncing…' : 'Sync now'}</button>}{unmatched?.synced_at && <span className="muted small">last sync {fmtDateTime(unmatched.synced_at)}</span>}</div>
            {sync && <div className="callout success small">{sync.fetched} transactions · {sync.matched} linked · {sync.created} added as expenses · {sync.unmatched} need you{sync.notes?.map((n, i) => <div key={i}>{n}</div>)}</div>}
            {unmatched && unmatched.unmatched.length > 0 && <div className="callout small"><b>Firefly fuel payments with no fill-up here:</b> log the fill-up with the litres and the same date and amount; the next sync links them.<ul style={{ margin: '6px 0 0', paddingLeft: 18 }}>{unmatched.unmatched.map(t => <li key={t.id}>{t.date} · {t.description} · {(t.amount / 100).toFixed(2)} {t.currency}</li>)}</ul></div>}
          </div>
        </Card>
        <Card title="🛰 Dawarich" actions={<span className={`badge ${overview.integrations.dawarich ? 'cat' : ''}`}>{overview.integrations.dawarich ? 'on' : settings?.dawarich_configured ? 'off' : 'not configured'}</span>}>
          <p className="muted small">Driving kilometres Dawarich records are added on top of your last odometer reading, so reminders by distance stay accurate between fill-ups. Switch it on for the vehicle you drive with the phone that records.</p>
          <label className="row"><input type="checkbox" checked={f.dawarich} onChange={e => setF({ ...f, dawarich: e.target.checked })} style={{ width: 'auto' }} disabled={!isOwner || !settings?.dawarich_configured} /> This vehicle drives the recorded kilometres{!isOwner && <span className="muted small"> (the owner's Dawarich; only the owner switches it)</span>}</label>
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
