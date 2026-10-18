import { createContext, useContext } from 'react'
import type { Overview, Vehicle } from '../../lib/types'

export interface VehicleCtx { vehicle: Vehicle; overview: Overview; reload: () => Promise<void>; canEdit: boolean }
export const VCtx = createContext<VehicleCtx>(null!)
export function useVehicle() { return useContext(VCtx) }
