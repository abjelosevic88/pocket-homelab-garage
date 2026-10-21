import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import type { MoneyStatus } from '../lib/types'
import { Select } from './Select'
import { Icon } from './icons'

/** Pocket Money status for the signed-in person; ?vehicle picks that vehicle's last account. */
export function useMoneyStatus(vehicleId?: number, refresh = '') {
  const [st, setSt] = useState<MoneyStatus | null>(null)
  useEffect(() => { api.get<MoneyStatus>(`${V1}/money${vehicleId ? `?vehicle=${vehicleId}` : ''}`).then(setSt).catch(() => setSt(null)) }, [vehicleId, refresh])
  return st
}

export interface MoneyChoice { money: boolean; money_account: number }

/**
 * "Create a transaction in Pocket Money" for a cost being logged. Shown when
 * Money is reachable (through Pocket Core or a token) and not switched off in
 * Settings; ticked by default for new entries.
 */
export function MoneyBook({ status, value, onChange, hasCost, initial = true }: { status: MoneyStatus | null; value: MoneyChoice | null; onChange: (c: MoneyChoice) => void; hasCost: boolean; initial?: boolean }) {
  useEffect(() => {
    if (status?.offer && !value) onChange({ money: initial, money_account: status.default_account })
  }, [status]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!status?.offer || !value) return null
  const accts = status.accounts
  return (
    <div className="money-book">
      <label className="row"><input type="checkbox" checked={value.money} onChange={e => onChange({ ...value, money: e.target.checked })} style={{ width: 'auto' }} /> Create a transaction in Pocket Money</label>
      {value.money && accts.length > 1 && <Select ariaLabel="Pocket Money account" value={String(value.money_account || '')} onChange={a => onChange({ ...value, money_account: Number(a) })} options={accts.map(a => ({ value: String(a.id), label: a.name, hint: a.currency }))} className="money-account" />}
      {value.money && !hasCost && <span className="muted small">needs a cost</span>}
    </div>
  )
}

/** What an already booked entry shows instead of the option. */
export function MoneyLinked({ status }: { status: MoneyStatus | null }) {
  return <div className="money-book muted small"><Icon name="wallet" size={15} />Booked in Pocket Money{status?.url && <> (<a href={status.url} target="_blank" rel="noreferrer">open</a>)</>}. Changes to the date, cost, description or vendor are carried over; deleting this deletes the transaction too.</div>
}

/** The body fields for a save; nothing when the option is not shown. */
export function moneyBody(c: MoneyChoice | null) {
  return c ? { money: c.money, money_account: c.money ? c.money_account || undefined : undefined } : {}
}

/** Tell the person when the save worked but booking did not. */
export function moneyWarning(r: { money_error?: string }, toast: (m: string, k?: 'ok' | 'err') => void) {
  if (r.money_error) toast(`Saved, but not booked in Pocket Money: ${r.money_error}`, 'err')
}
