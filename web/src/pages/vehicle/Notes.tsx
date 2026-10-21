import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import type { Note } from '../../lib/types'
import { ago } from '../../lib/format'
import { Confirm, Empty, Loading, useAsync } from '../../components/ui'
import { NoteModal } from '../../components/forms'
import { useVehicle } from './ctx'
import { Icon } from '../../components/icons'

export default function Notes() {
  const { vehicle: v, reload: reloadOv, canEdit } = useVehicle()
  const { data, loading, reload } = useAsync(() => api.get<Note[]>(`${V1}/vehicles/${v.id}/notes`), [v.id])
  const [edit, setEdit] = useState<Note | null | 'new'>(null)
  const [del, setDel] = useState<Note | null>(null)
  const pin = async (n: Note) => { await api.put(`${V1}/vehicles/${v.id}/notes/${n.id}`, { pinned: !n.pinned }); reload(); reloadOv() }
  if (loading && !data) return <Loading />
  return (
    <>
      <div className="toolbar"><span className="muted small">Tyre pressures, radio codes, which oil, where the spare key is. Pinned notes show on the overview.</span><span className="spacer" />{canEdit && <button className="btn primary sm" onClick={() => setEdit('new')}>+ Note</button>}</div>
      {!data || data.length === 0 ? <Empty title="No notes yet" /> : (
        <div className="notes-grid">{data.map(n => (
          <div key={n.id} className={`note-card ${n.pinned ? 'pinned' : ''}`}>
            <div className="row" style={{ justifyContent: 'space-between' }}><b>{n.title || <span className="muted">Untitled</span>}</b>{canEdit && <span className="row" style={{ gap: 2 }}><button className="btn ghost sm" onClick={() => pin(n)} title={n.pinned ? 'Unpin' : 'Pin'} aria-label={n.pinned ? 'Unpin' : 'Pin'}><Icon name="pin" size={14} className={n.pinned ? 'ok-text' : ''} /></button><button className="btn ghost sm" onClick={() => setEdit(n)}>Edit</button><button className="btn ghost sm icon-only danger-text" onClick={() => setDel(n)} aria-label="Delete" title="Delete"><Icon name="trash" size={14} /></button></span>}</div>
            <pre>{n.body}</pre>
            <div className="muted small mt-sm">{ago(n.updated_at)}</div>
          </div>
        ))}</div>
      )}
      {edit && <NoteModal vehicle={v} note={edit === 'new' ? undefined : edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload(); reloadOv() }} />}
      {del && <Confirm title="Delete this note?" message="Gone for good." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/vehicles/${v.id}/notes/${del.id}`); setDel(null); reload(); reloadOv() }} />}
    </>
  )
}
