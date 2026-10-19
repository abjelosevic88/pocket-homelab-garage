import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { Rec } from '../../lib/types'
import { KIND_ICON, KIND_LABEL } from '../../lib/types'
import { fmtDate } from '../../lib/format'
import { cleanNotes, fmtOdo } from '../../lib/units'
import { Card, Confirm, Empty, Loading, Segmented, useAsync, useToast } from '../../components/ui'
import { Money } from '../../components/Money'
import { RecordModal } from '../../components/forms'
import { Select } from '../../components/Select'
import { useVehicle } from './ctx'
import { useMoneyStatus } from '../../components/MoneyBook'

export default function ServiceTab() {
  const { vehicle: v, reload: reloadOv, canEdit, overview } = useVehicle()
  const { meta } = useApp()
  const { toast } = useToast()
  const [kind, setKind] = useState('')
  const [q, setQ] = useState('')
  const [year, setYear] = useState('')
  const { data, loading, reload } = useAsync(() => api.get<Rec[]>(`${V1}/vehicles/${v.id}/records?kind=${kind}&q=${encodeURIComponent(q)}${year ? `&from=${year}-01-01&to=${year}-12-31` : ''}`), [v.id, kind, q, year])
  const [edit, setEdit] = useState<Rec | null | 'new'>(null)
  const [del, setDel] = useState<Rec | null>(null)
  const [open, setOpen] = useState<number | null>(null)
  const list = data || []
  const total = list.reduce((a, r) => a + r.cost_home, 0)
  const years = Array.from(new Set((overview.months.map(m => m.month.slice(0, 4))).concat(list.map(r => r.date.slice(0, 4))))).sort().reverse()
  const done = () => { setEdit(null); reload(); reloadOv() }
  const ms = useMoneyStatus(v.id)
  const book = async (r: Rec) => { try { await api.post(`${V1}/vehicles/${v.id}/records/${r.id}/money`, {}); toast('Booked in Pocket Money'); reload() } catch (e) { toast((e as Error).message, 'err') } }
  const push = async (r: Rec) => { try { await api.post(`${V1}/vehicles/${v.id}/records/${r.id}/firefly`); toast('Sent to Firefly'); reload() } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <>
      <div className="toolbar">
        <Segmented value={kind} onChange={setKind} options={[{ value: '', label: 'All' }, ...(meta?.record_kinds || []).map(k => ({ value: k, label: KIND_LABEL[k] }))]} size="sm" />
        <div className="search"><span className="ico">⌕</span><input value={q} onChange={e => setQ(e.target.value)} placeholder="Search description, vendor, parts…" /></div>
        <Select ariaLabel="Year" value={year} onChange={setYear} options={years.map(y => ({ value: String(y), label: String(y) }))} placeholder="All years" clearLabel="All years" />
        <a className="btn sm" href={`${V1}/vehicles/${v.id}/export/records.csv?kind=${kind}`}>CSV</a>
        {canEdit && <button className="btn primary sm" onClick={() => setEdit('new')}>+ Log work</button>}
      </div>
      <Card title={<>{list.length} {kind ? KIND_LABEL[kind].toLowerCase() : 'record'}{list.length === 1 ? '' : 's'} <span className="muted">· <Money minor={total} code={v.currency} /></span></>} flush>
        {loading && !data ? <Loading /> : list.length === 0 ? <div className="pad"><Empty title={q || kind || year ? 'No matches' : 'Nothing logged yet'}>{!q && !kind && 'Service, repairs, upgrades and plain expenses (registration, insurance, washes) all live here.'}</Empty></div> : (
          <table className="table compact">
            <thead><tr><th>Date</th><th>Kind</th><th>What</th><th className="num">Odometer</th><th className="num">Cost</th><th /></tr></thead>
            <tbody>{list.map(r => (<>
              <tr key={r.id} onClick={() => setOpen(open === r.id ? null : r.id)} style={{ cursor: 'pointer' }}>
                <td style={{ whiteSpace: 'nowrap' }}>{fmtDate(r.date)}</td>
                <td><span className="badge cat">{KIND_ICON[r.kind]} {KIND_LABEL[r.kind]}</span>{r.category && <span className="badge" style={{ marginLeft: 4 }}>{r.category}</span>}</td>
                <td><b>{r.description}</b>{r.vendor && <span className="muted small"> · {r.vendor}</span>}{(r.parts?.length || 0) > 0 && <span className="muted small"> · {r.parts.length} part{r.parts.length === 1 ? '' : 's'}</span>}{r.firefly_id && <span className="tag-src"> · firefly</span>}{r.money_tx_id && <span className="tag-src" title="Booked in Pocket Money"> · money</span>}{r.source !== 'manual' && r.source !== 'firefly' && <span className="tag-src"> · {r.source}</span>}</td>
                <td className="num muted">{r.odometer ? fmtOdo(r.odometer, v.odometer_unit, false) : ''}</td>
                <td className="num">{r.cost > 0 ? <><Money minor={r.cost} code={r.currency} />{r.currency !== v.currency && <div className="muted small"><Money minor={r.cost_home} code={v.currency} /></div>}</> : <span className="muted">—</span>}</td>
                <td className="right" style={{ whiteSpace: 'nowrap' }} onClick={e => e.stopPropagation()}>{canEdit && <>{ms?.connected && !r.money_tx_id && r.cost > 0 && <button className="btn ghost sm" onClick={() => book(r)} title="Create the transaction in Pocket Money">→ Money</button>}{!ms?.offer && v.role === 'owner' && overview.integrations.firefly && !r.firefly_id && r.cost > 0 && <button className="btn ghost sm" onClick={() => push(r)} title="Create the withdrawal in Firefly III">→ Firefly</button>}<button className="btn ghost sm" onClick={() => setEdit(r)}>Edit</button><button className="btn ghost sm danger-text" onClick={() => setDel(r)}>✕</button></>}</td>
              </tr>
              {open === r.id && (cleanNotes(r.notes) || (r.parts?.length || 0) > 0 || r.tags) && <tr key={`${r.id}-d`}><td colSpan={6} style={{ background: 'var(--surface-2)' }}>
                {cleanNotes(r.notes) && <div className="small" style={{ whiteSpace: 'pre-wrap' }}>{cleanNotes(r.notes)}</div>}
                {(r.parts?.length || 0) > 0 && <table className="table compact mt-sm" style={{ maxWidth: 600 }}><thead><tr><th>Part</th><th className="num">Qty</th><th>Part no.</th><th className="num">Cost</th></tr></thead><tbody>{r.parts.map((p, i) => <tr key={i}><td>{p.link ? <a href={p.link} target="_blank" rel="noreferrer">{p.name}</a> : p.name}</td><td className="num">{p.qty}</td><td className="mono small">{p.part_no}</td><td className="num">{p.cost > 0 && <Money minor={p.cost} code={r.currency} />}</td></tr>)}</tbody></table>}
                {r.tags && <div className="chips mt-sm">{r.tags.split(/[\s,]+/).filter(Boolean).map(t => <span key={t} className="badge tag">{t}</span>)}</div>}
              </td></tr>}
            </>))}</tbody>
          </table>
        )}
      </Card>
      {edit && <RecordModal vehicle={v} record={edit === 'new' ? undefined : edit} kind={kind || 'service'} onClose={() => setEdit(null)} onSaved={done} />}
      {del && <Confirm title={`Delete “${del.description}”?`} message={`Documents attached to it stay in the Documents tab.${del.money_tx_id ? ' Its Pocket Money transaction is deleted too.' : ''}`} onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/vehicles/${v.id}/records/${del.id}`); setDel(null); reload(); reloadOv() }} />}
    </>
  )
}
