// SPDX-License-Identifier: AGPL-3.0-only
import { inject, type ComputedRef, type InjectionKey, type Ref } from 'vue'
import type { Approval } from './agents'
import type { ActionKey, Journey, PluginInfo, ReleaseRef, Stage } from './journey'
import type { JourneyData } from './useJourneyData'
import type { Plan } from './usePlan'

// What every Journey stage reads and can do, provided by the Journey view.
export interface NextState { label: string; disabled: boolean; tip: string; busy: boolean }
export interface JourneyContext {
  project: ComputedRef<{ id: string; routeKey: string; title: string }>
  journey: ComputedRef<Journey>
  data: JourneyData
  plan: Plan
  release: ComputedRef<ReleaseRef | null>
  releaseLabel: ComputedRef<string>
  current: ComputedRef<boolean>
  editable: ComputedRef<boolean>
  canAct: ComputedRef<boolean>
  me: ComputedRef<string | null>
  now: Ref<number>
  approvals: ComputedRef<Approval[]>
  plugins: Ref<PluginInfo[]>
  next: ComputedRef<NextState>
  runNext: () => void
  act: (action: ActionKey, options?: { approval?: Approval | null; reason?: string; done?: string }) => Promise<boolean>
  view: (stage: Stage) => void
  walk: (key?: string) => void
  open: (key: string) => void
  selectRelease: (key: string | null) => void
}
export const JOURNEY: InjectionKey<JourneyContext> = Symbol('journey')
export function useJourneyContext(): JourneyContext {
  const context = inject(JOURNEY)
  if (!context) throw new Error('Journey stages need the Journey view.')
  return context
}
