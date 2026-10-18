import type { Settings } from './types'

export type Units = Pick<Settings, 'distance_unit' | 'volume_unit' | 'economy_unit'>

const LITRES_PER = { l: 1, gal_us: 3.785411784, gal_uk: 4.54609 }
export const VOL_LABEL: Record<string, string> = { l: 'L', gal_us: 'gal', gal_uk: 'gal' }
export const ECON_LABEL: Record<string, string> = { l100: 'L/100 km', kml: 'km/L', mpg_us: 'mpg', mpg_uk: 'mpg (UK)' }
export const ECON_OPTIONS = [{ id: 'l100', label: 'L/100 km' }, { id: 'kml', label: 'km/L' }, { id: 'mpg_us', label: 'mpg (US)' }, { id: 'mpg_uk', label: 'mpg (UK)' }]
export const VOL_OPTIONS = [{ id: 'l', label: 'Litres' }, { id: 'gal_us', label: 'US gallons' }, { id: 'gal_uk', label: 'UK gallons' }]

export function fmtNum(n: number, d = 0): string { return n.toLocaleString(undefined, { minimumFractionDigits: d, maximumFractionDigits: d }) }
/** Odometer readings are stored in the vehicle's own unit. */
export function fmtOdo(n: number, unit: string, withUnit = true): string { return fmtNum(Math.round(n)) + (withUnit ? ` ${unit}` : '') }
/** Litres stored → display volume. */
export function toVol(l: number, u: Units): number { return l / LITRES_PER[u.volume_unit] }
export function fromVol(v: number, u: Units): number { return v * LITRES_PER[u.volume_unit] }
export function fmtVol(l: number, u: Units, d = 2): string { return `${fmtNum(toVol(l, u), d)} ${VOL_LABEL[u.volume_unit]}` }
/** economy is stored as volume per 100 distance-units (litres per 100 km when the vehicle is metric). */
export function toEcon(l100: number, u: Units, odoUnit: string): number {
  if (!l100) return 0
  switch (u.economy_unit) {
    case 'kml': return 100 / l100
    case 'mpg_us': { const kmPerL = 100 / l100; return odoUnit === 'mi' ? (100 / l100) * 3.785411784 : kmPerL * 2.352145833 }
    case 'mpg_uk': { const kmPerL = 100 / l100; return odoUnit === 'mi' ? (100 / l100) * 4.54609 : kmPerL * 2.824809363 }
    default: return odoUnit === 'mi' ? l100 / 1.609344 : l100
  }
}
export function fmtEcon(l100: number, u: Units, odoUnit: string): string {
  if (!l100) return '—'
  const v = toEcon(l100, u, odoUnit)
  return `${fmtNum(v, v < 10 ? 2 : 1)} ${u.economy_unit === 'l100' ? (odoUnit === 'mi' ? 'L/100 mi' : 'L/100 km') : ECON_LABEL[u.economy_unit]}`
}
/** price per litre in minor units → "1.54 €/L". */
export function fmtPricePer(minorPerL: number, code: string, decimals: number, u: Units): string {
  const per = minorPerL * LITRES_PER[u.volume_unit] / Math.pow(10, decimals)
  try { return `${new Intl.NumberFormat(undefined, { style: 'currency', currency: code, currencyDisplay: 'narrowSymbol', minimumFractionDigits: 2, maximumFractionDigits: 3 }).format(per)}/${VOL_LABEL[u.volume_unit]}` } catch { return `${per.toFixed(3)} ${code}/${VOL_LABEL[u.volume_unit]}` }
}
export function cleanNotes(s: string): string { return (s || '').split('\n').filter(l => !l.trim().startsWith('[Pocket Homelab')).join('\n').trim() }
export function daysWord(n: number): string { return n === 1 ? '1 day' : `${n} days` }
