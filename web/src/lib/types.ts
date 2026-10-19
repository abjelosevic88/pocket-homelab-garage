export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }
export interface Currency { code: string; name: string; decimals: number }

export interface Settings {
  title: string; home_currency: string; date_format: string; time_format: string
  distance_unit: 'km' | 'mi'; volume_unit: 'l' | 'gal_us' | 'gal_uk'; economy_unit: 'l100' | 'kml' | 'mpg_us' | 'mpg_uk'
  fx_auto: boolean; fx_provider: string
  firefly_url: string; firefly_token: string; firefly_configured: boolean; firefly_category: string; firefly_account: string; firefly_auto_sync: boolean
  dawarich_url: string; dawarich_api_key: string; dawarich_configured: boolean
  notify_url: string; notify_format: string; notify_hour: number; notify_digest: boolean; notify_configured: boolean
  travel_url: string; travel_public_url: string; travel_token: string; travel_configured: boolean
  inventory_url: string; inventory_token: string; inventory_configured: boolean; inventory_take: boolean
  core_configured: boolean
  travel_mode: ConnMode; travel_via: '' | 'core' | 'direct'; travel_detail: string
  inventory_mode: ConnMode; inventory_via: '' | 'core' | 'direct'; inventory_detail: string
  money_mode: ConnMode; money_via: '' | 'core' | 'direct'; money_detail: string
  money_enabled: boolean; money_url: string; money_token: string; money_account: number; money_category_fuel: number; money_category: number
  max_upload_bytes: number; setup_complete: boolean; base_url: string
}

export interface Meta {
  vehicle_kinds: string[]; fuel_types: string[]; record_kinds: string[]; plan_stages: string[]; priorities: string[]
  document_kinds: string[]; service_categories: string[]; expense_categories: string[]
  reminder_templates: { title: string; kind: string; category: string; interval_days: number; interval_km: number }[]
}

export interface VehicleMember { user_id: number; name: string; email: string; role: 'owner' | 'editor' | 'viewer' }

export interface Summary {
  odometer: number; odometer_date: string; odometer_estimate: number; estimate_km: number
  cost_total: number; cost_year: number; cost_month: number; cost_fuel_total: number; cost_by_kind: Record<string, number>
  fills: number; records: number; reminders: number; reminders_due: number; reminders_soon: number; next_reminder?: Reminder
  economy: number; economy_recent: number; cost_per_km: number; fuel_per_km: number; km_per_year: number
  last_fuel?: Fuel; last_service?: Rec; owned_days: number; plans: number
}

export interface Vehicle {
  id: number; owner_id: number; name: string; year: number; make: string; model: string; trim: string; plate: string; vin: string
  kind: string; fuel_type: string; odometer_unit: 'km' | 'mi' | 'h'; tank_capacity: number; currency: string; color: string; has_image: boolean
  purchase_date: string; purchase_price: number; purchase_odo: number; sold_date: string; sold_price: number
  status: 'active' | 'sold' | 'archived'; odometer_offset: number; notes: string; tags: string
  firefly_tag: string; firefly_account: string; dawarich: boolean; sort_order: number; external_id: string
  created_at: string; updated_at: string; display_name: string; role?: string; members?: VehicleMember[]; summary?: Summary
}

export interface Part { name: string; qty: number; cost: number; part_no: string; inventory_id?: number; link?: string }

export interface Rec {
  id: number; vehicle_id: number; kind: 'service' | 'repair' | 'upgrade' | 'expense'; category: string; date: string; odometer: number | null
  description: string; notes: string; vendor: string; cost: number; currency: string; cost_home: number; fx_rate: number; parts: Part[]
  tags: string; source: string; external_id: string; firefly_id: string; created_at: string; updated_at: string
  money_tx_id: number | null; money_error?: string; inventory_error?: string
}

export interface Fuel {
  id: number; vehicle_id: number; date: string; odometer: number; volume: number; cost: number; currency: string; cost_home: number; fx_rate: number
  full_tank: boolean; missed: boolean; station: string; grade: string; soc_start: number; soc_end: number; lat: number | null; lng: number | null
  notes: string; tags: string; source: string; firefly_id: string; created_at: string
  money_tx_id: number | null; money_error?: string
  distance: number; economy: number; price_per: number; cost_per_km: number
}

export interface OdometerReading { id: number; vehicle_id: number; date: string; reading: number; notes: string; source: string; link_kind?: string; link_id?: number; created_at: string; delta: number; days: number }

export interface Reminder {
  id: number; vehicle_id: number; title: string; notes: string; kind: string; category: string; due_date: string; due_odometer: number | null
  interval_days: number; interval_km: number; soon_days: number; soon_km: number; status: 'active' | 'done' | 'dismissed'
  last_done_date: string; last_done_odo: number | null; urgency: 'ok' | 'soon' | 'due' | 'overdue' | 'done'; days_left: number | null; km_left: number | null
  vehicle_name?: string; created_at: string
}

export interface Plan { id: number; vehicle_id: number; title: string; notes: string; kind: string; category: string; priority: string; stage: string; estimate: number; target_date: string; record_id: number | null; sort_order: number }
export interface Note { id: number; vehicle_id: number; title: string; body: string; pinned: boolean; created_at: string; updated_at: string }
export interface Doc { id: number; vehicle_id: number; name: string; mime: string; bytes: number; kind: string; expires_on: string; link_kind: string; link_id: number; created_at: string }
export interface Activity { id: number; user_name: string; action: string; subject: string; created_at: string }
export interface MonthlyCost { month: string; total: number; by_kind: Record<string, number> }
export interface YearlyCost { year: string; total: number; by_kind: Record<string, number>; km: number; fills: number; volume: number }
export interface TripLink { id: number; name: string; destination: string; start_date: string; end_date: string; color: string; fuel_cost: number; other_cost: number; litres: number; fills: number; km: number; link: string }

export interface Overview {
  vehicle: Vehicle; reminders: Reminder[]; recent_records: Rec[]; recent_fuel: Fuel[]; open_plans: number; pinned_notes: Note[]
  expiring_documents: Doc[]; months: MonthlyCost[]; activity: Activity[]; trips: TripLink[]
  integrations: { firefly: boolean; dawarich: boolean; travel: boolean; inventory: boolean; notify: boolean }
}

export interface HomeData {
  stats: { vehicles: number; active: number; cost_year: number; cost_month: number; due: number; soon: number }
  vehicles: Vehicle[]; reminders: Reminder[]; currency: string
  recent: { kind: string; id: number; vehicle_id: number; vehicle_name: string; date: string; title: string; cost: number; currency: string; odometer: number }[]
}

export const KIND_LABEL: Record<string, string> = { service: 'Service', repair: 'Repair', upgrade: 'Upgrade', expense: 'Expense', fuel: 'Fuel' }
export const KIND_ICON: Record<string, string> = { service: '🔧', repair: '🛠', upgrade: '✨', expense: '🧾', fuel: '⛽', odometer: '🧭' }
export const VEHICLE_ICON: Record<string, string> = { car: '🚗', motorcycle: '🏍', van: '🚐', truck: '🚚', camper: '🚙', bike: '🚲', boat: '⛵', tractor: '🚜', other: '🛞' }
export const URGENCY_LABEL: Record<string, string> = { overdue: 'Overdue', due: 'Due', soon: 'Soon', ok: 'Scheduled', done: 'Done' }
export const STAGE_LABEL: Record<string, string> = { backlog: 'Backlog', planned: 'Planned', in_progress: 'In progress', testing: 'Testing', done: 'Done' }
/** Chart series colours — the validated categorical palette (slots 1–5), light and dark steps. */
export const SERIES: Record<string, { light: string; dark: string }> = {
  fuel: { light: '#2a78d6', dark: '#3987e5' }, service: { light: '#eb6834', dark: '#d95926' }, repair: { light: '#1baf7a', dark: '#199e70' },
  expense: { light: '#eda100', dark: '#c98500' }, upgrade: { light: '#e87ba4', dark: '#d55181' },
}
export const SERIES_ORDER = ['fuel', 'service', 'repair', 'expense', 'upgrade']

/** Pocket Money as seen by the signed-in person (GET /api/v1/money). */
export interface MoneyStatus {
  connected: boolean; via: '' | 'core' | 'token'; error: string; enabled: boolean; offer: boolean; core: boolean; url: string
  accounts: { id: number; name: string; currency: string; kind: string; everyday: boolean }[]; default_account: number
}

/** How Garage reaches a sibling Pocket app. */
export type ConnMode = 'core' | 'direct' | 'off'
