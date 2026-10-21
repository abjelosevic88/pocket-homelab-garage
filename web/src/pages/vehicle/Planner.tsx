import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { Plan } from '../../lib/types'
import { STAGE_LABEL } from '../../lib/types'
import { fmtDate } from '../../lib/format'
import { Confirm, Loading, useAsync, useToast } from '../../components/ui'
import { Money } from '../../components/Money'
import { PlanDoneModal, PlanModal } from '../../components/forms'
import { useVehicle } from './ctx'
import { Icon, LOG_KIND_ICON } from '../../components/icons'

export default function Planner() {
  const { vehicle: v, reload: reloadOv, canEdit } = useVehicle()
  const { meta } = useApp()
  const { toast } = useToast()
  const { data, loading, reload, setData } = useAsync(() => api.get<Plan[]>(`${V1}/vehicles/${v.id}/plans`), [v.id])
  const [edit, setEdit] = useState<Plan | null>(null)
  const [create, setCreate] = useState<string | null>(null)
  const [finish, setFinish] = useState<Plan | null>(null)
  const [del, setDel] = useState<Plan | null>(null)
  const [drag, setDrag] = useState<number | null>(null)
  const [over, setOver] = useState<string | null>(null)
  const stages = meta?.plan_stages || []
  const plans = data || []
  const move = async (id: number, stage: string) => {
    const col = plans.filter(p => p.stage === stage && p.id !== id).map(p => p.id)
    col.push(id)
    setData(plans.map(p => p.id === id ? { ...p, stage } : p))
    try { setData(await api.put<Plan[]>(`${V1}/vehicles/${v.id}/plans/reorder`, { stage, ids: col })); reloadOv() } catch (e) { toast((e as Error).message, 'err'); reload() }
  }
  const estimate = plans.filter(p => p.stage !== 'done').reduce((a, p) => a + p.estimate, 0)
  if (loading && !data) return <Loading />
  return (
    <>
      <div className="toolbar"><span className="muted small">Work you intend to do. Drag cards between stages; “Log & finish” turns one into a record with the real cost.{estimate > 0 && <> Open estimates: <b><Money minor={estimate} code={v.currency} /></b>.</>}</span><span className="spacer" />{canEdit && <button className="btn primary sm" onClick={() => setCreate('backlog')}>+ Plan work</button>}</div>
      <div className="kanban">
        {stages.map(st => {
          const col = plans.filter(p => p.stage === st)
          return (
            <div key={st} className={`col ${over === st ? 'over' : ''}`} onDragOver={e => { if (canEdit) { e.preventDefault(); setOver(st) } }} onDragLeave={() => setOver(null)} onDrop={e => { e.preventDefault(); setOver(null); if (drag != null) move(drag, st); setDrag(null) }}>
              <h4><span>{STAGE_LABEL[st]}</span><span>{col.length}</span></h4>
              {col.map(p => (
                <div key={p.id} className={`kcard ${drag === p.id ? 'drag' : ''}`} draggable={canEdit} onDragStart={() => setDrag(p.id)} onDragEnd={() => setDrag(null)} onClick={() => canEdit && setEdit(p)}>
                  <div className="row" style={{ gap: 6, alignItems: 'flex-start' }}><span className={`prio ${p.priority}`} title={p.priority} style={{ marginTop: 6 }} /><b style={{ flex: 1 }}>{p.title}</b><Icon name={LOG_KIND_ICON[p.kind] || 'wrench'} size={14} className="muted" title={p.kind} /></div>
                  <div className="meta">{p.category && <span className="badge">{p.category}</span>}{p.estimate > 0 && <Money minor={p.estimate} code={v.currency} />}{p.target_date && <span>by {fmtDate(p.target_date, false)}</span>}</div>
                  {canEdit && st !== 'done' && <div className="row mt-sm" style={{ gap: 4 }} onClick={e => e.stopPropagation()}><button className="btn sm primary" onClick={() => setFinish(p)}>Log & finish</button>{st !== 'in_progress' && <button className="btn sm ghost" onClick={() => move(p.id, 'in_progress')}>Start</button>}<button className="btn sm ghost icon-only danger-text" onClick={() => setDel(p)} aria-label="Delete" title="Delete"><Icon name="trash" size={14} /></button></div>}
                </div>
              ))}
              {canEdit && <button className="btn ghost sm" style={{ width: '100%' }} onClick={() => setCreate(st)}>+ add</button>}
            </div>
          )
        })}
      </div>
      {create && <PlanModal vehicle={v} stage={create} onClose={() => setCreate(null)} onSaved={() => { setCreate(null); reload(); reloadOv() }} />}
      {edit && <PlanModal vehicle={v} plan={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload() }} />}
      {finish && <PlanDoneModal vehicle={v} plan={finish} onClose={() => setFinish(null)} onDone={() => { setFinish(null); reload(); reloadOv() }} />}
      {del && <Confirm title={`Delete “${del.title}”?`} message="The card is removed from the board." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/vehicles/${v.id}/plans/${del.id}`); setDel(null); reload(); reloadOv() }} />}
    </>
  )
}
