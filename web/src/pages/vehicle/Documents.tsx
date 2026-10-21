import { useRef, useState } from 'react'
import { api, uploadForm, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { Doc } from '../../lib/types'
import { fmtBytes, fmtDate } from '../../lib/format'
import { Confirm, Empty, Field, Loading, Modal, useAsync, useToast } from '../../components/ui'
import { Select } from '../../components/Select'
import { useVehicle } from './ctx'
import { Icon } from '../../components/icons'

const cap = (s: string) => s ? s[0].toUpperCase() + s.slice(1) : ''

export default function Documents() {
  const { vehicle: v, reload: reloadOv, canEdit } = useVehicle()
  const { meta } = useApp()
  const { toast } = useToast()
  const { data, loading, reload } = useAsync(() => api.get<Doc[]>(`${V1}/vehicles/${v.id}/documents`), [v.id])
  const [edit, setEdit] = useState<Doc | null>(null)
  const [del, setDel] = useState<Doc | null>(null)
  const [overDrop, setOver] = useState(false)
  const [busy, setBusy] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const kinds = meta?.document_kinds || []
  const upload = async (files: FileList | File[]) => {
    setBusy(true)
    try { for (const f of Array.from(files)) await uploadForm(`${V1}/vehicles/${v.id}/documents`, f, { name: f.name.replace(/\.[^.]+$/, ''), kind: guess(f.name) }); toast('Uploaded'); reload(); reloadOv() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const today = new Date().toISOString().slice(0, 10)
  if (loading && !data) return <Loading />
  return (
    <>
      <div className={`toolbar dropzone ${overDrop ? 'over' : ''}`} onDragOver={e => { e.preventDefault(); setOver(true) }} onDragLeave={() => setOver(false)} onDrop={e => { e.preventDefault(); setOver(false); if (canEdit) upload(e.dataTransfer.files) }}>
        <span className="muted small">Registration, insurance policy, invoices, the manual — with an expiry date so you get warned. Drop files here.</span><span className="spacer" />
        {canEdit && <><input ref={input} type="file" multiple hidden onChange={e => e.target.files && upload(e.target.files)} /><button className="btn primary sm" onClick={() => input.current?.click()} disabled={busy}>{busy ? 'Uploading…' : '+ Upload'}</button></>}
      </div>
      {!data || data.length === 0 ? <Empty title="No documents yet" /> : (
        <div className="doc-grid">{data.map(d => (
          <div key={d.id} className="doc-card">
            <div className="row" style={{ justifyContent: 'space-between' }}><span className="ext">{ext(d)}</span><span className="badge">{cap(d.kind)}</span></div>
            <a href={`${V1}/vehicles/${v.id}/documents/${d.id}/file`} target="_blank" rel="noreferrer"><b>{d.name}</b></a>
            <div className="muted small">{fmtBytes(d.bytes)} · {fmtDate(d.created_at, false)}{d.expires_on && <> · {d.expires_on < today ? <span className="bad">expired {fmtDate(d.expires_on)}</span> : <>valid until {fmtDate(d.expires_on)}</>}</>}</div>
            {canEdit && <div className="row" style={{ gap: 2 }}><a className="btn ghost sm" href={`${V1}/vehicles/${v.id}/documents/${d.id}/file?download=1`}>Download</a><button className="btn ghost sm" onClick={() => setEdit(d)}>Edit</button><button className="btn ghost sm icon-only danger-text" onClick={() => setDel(d)} aria-label="Delete" title="Delete"><Icon name="trash" size={14} /></button></div>}
          </div>
        ))}</div>
      )}
      {edit && <DocModal doc={edit} kinds={kinds} vehicleId={v.id} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload(); reloadOv() }} />}
      {del && <Confirm title={`Delete “${del.name}”?`} message="The file is removed from the server." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/vehicles/${v.id}/documents/${del.id}`); setDel(null); reload(); reloadOv() }} />}
    </>
  )
}

function ext(d: Doc): string { const m = d.mime.split('/')[1] || ''; return m.replace('vnd.openxmlformats-officedocument.wordprocessingml.document', 'docx').replace('jpeg', 'jpg').slice(0, 4) || 'file' }
function guess(name: string): string {
  const n = name.toLowerCase()
  if (/polis|insur|osigur|kasko/.test(n)) return 'insurance'
  if (/regist|saobra/.test(n)) return 'registration'
  if (/racun|račun|invoice|faktura/.test(n)) return 'invoice'
  if (/manual|uputstvo/.test(n)) return 'manual'
  if (/tehni|inspect/.test(n)) return 'inspection'
  if (/\.(jpe?g|png|heic|webp)$/.test(n)) return 'photo'
  return 'other'
}

function DocModal({ doc, kinds, vehicleId, onClose, onSaved }: { doc: Doc; kinds: string[]; vehicleId: number; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: doc.name, kind: doc.kind, expires_on: doc.expires_on })
  const save = async (e: React.FormEvent) => { e.preventDefault(); try { await api.put(`${V1}/vehicles/${vehicleId}/documents/${doc.id}`, f); toast('Saved'); onSaved() } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Modal title="Document" onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus /></Field>
        <div className="two-col tight">
          <Field label="Kind"><Select value={f.kind} onChange={v => setF({ ...f, kind: v })} options={kinds.map(k => ({ value: k, label: cap(k) }))} /></Field>
          <Field label="Valid until" help="Insurance and registration: you are warned 30 days before."><input type="date" value={f.expires_on} onChange={e => setF({ ...f, expires_on: e.target.value })} /></Field>
        </div>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Save</button></div>
      </form>
    </Modal>
  )
}
