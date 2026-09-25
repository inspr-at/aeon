// SPDX-License-Identifier: AGPL-3.0-only
// The graph API never sends bodies. Force libraries mutate their inputs, so every
// renderer receives its own layout copy; URL and Vue state retain the wire data.
import { api } from './api.ts'
import { type KnowledgeType, type KnowledgeStatus, typeMeta } from './knowledge.ts'

export interface GraphNode {
  id: string; key: string; type: KnowledgeType | 'ticket'; slug: string; title: string
  status: string; degree: number; updated_at: string; kind: 'knowledge' | 'ticket'
}
export interface GraphEdge { source: string; target: string; kind: 'relation' | 'mention'; label: string }
export interface KnowledgeGraphData { nodes: GraphNode[]; edges: GraphEdge[]; truncated: boolean }
export interface LayoutNode extends GraphNode { x?: number; y?: number; z?: number; vx?: number; vy?: number; vz?: number }
export interface LayoutEdge extends Omit<GraphEdge, 'source' | 'target'> { source: string | LayoutNode; target: string | LayoutNode }
export type GraphDimension = '2d' | '3d'
export const endpointID = (endpoint: string | LayoutNode) => typeof endpoint === 'string' ? endpoint : endpoint.id
export const graphEntry = (node: GraphNode) => node.kind === 'knowledge' ? `${node.type}/${node.slug}` : ''
export const graphTypeLabel = (node: GraphNode) => node.kind === 'ticket' ? 'Ticket' : typeMeta(node.type).label
export const graphRadius = (node: GraphNode) => 8 * Math.sqrt(node.degree + 1)

// Palette tokens already used by Aeon's type/status icons. Resolve at render
// time, including theme changes; no baked-in light-mode colours or remote assets.
export const graphTypeTokens: Record<GraphNode['type'], string> = {
  runbook: '--teal', guideline: '--ok', memory: '--gold-ink',
  'external-system': '--ink-2', 'related-project': '--st-new', ticket: '--st-backlog',
}
export interface GraphPalette { background: string; ink: string; muted: string; dark: boolean; colors: Record<string, string>; font: string }
export function graphPalette(): GraphPalette {
  const css = getComputedStyle(document.documentElement)
  const read = (token: string) => css.getPropertyValue(token).trim()
  return { background: read('--canvas'), ink: read('--ink'), muted: read('--ink-3'), font: read('--font'),
    dark: css.colorScheme === 'dark', colors: Object.fromEntries(Object.entries(graphTypeTokens).map(([type, token]) => [type, read(token)])) }
}
export async function fetchKnowledgeGraph(project: string, statuses: KnowledgeStatus[], tickets: boolean, signal: AbortSignal): Promise<KnowledgeGraphData> {
  const params = new URLSearchParams({ project_id: project })
  if (statuses.length) params.set('status', statuses.join(','))
  if (tickets) params.set('include', 'tickets')
  const response = await api(`/knowledge/graph?${params}`, { signal: AbortSignal.any([signal, AbortSignal.timeout(20_000)]) })
  if (!response.ok) throw new Error('The graph could not be loaded. Please try again.')
  return response.json()
}
// Type filters keep only satellites attached to a retained entry. Search is a
// highlight, so finding one entry never destroys the context around it.
export function filterGraph(data: KnowledgeGraphData, type: KnowledgeType | ''): KnowledgeGraphData {
  if (!type) return data
  const entries = new Set(data.nodes.filter(n => n.kind === 'knowledge' && n.type === type).map(n => n.id))
  const tickets = new Set(data.nodes.filter(n => n.kind === 'ticket').map(n => n.id))
  const visible = new Set(entries)
  for (const edge of data.edges) {
    if (entries.has(edge.source) && tickets.has(edge.target)) visible.add(edge.target)
    if (entries.has(edge.target) && tickets.has(edge.source)) visible.add(edge.source)
  }
  return { nodes: data.nodes.filter(n => visible.has(n.id)), edges: data.edges.filter(e => visible.has(e.source) && visible.has(e.target)), truncated: data.truncated }
}
export function graphMatches(nodes: GraphNode[], query: string): Set<string> {
  const words = query.toLowerCase().trim().split(/\s+/).filter(Boolean)
  return new Set(nodes.filter(n => words.every(word => `${n.title} ${n.slug} ${n.key} ${n.type}`.toLowerCase().includes(word))).map(n => n.id))
}
export function graphNeighbours(edges: GraphEdge[], id: string): Set<string> {
  const ids = new Set<string>([id])
  for (const edge of edges) { if (edge.source === id) ids.add(edge.target); if (edge.target === id) ids.add(edge.source) }
  return ids
}
export function graphLayout(data: KnowledgeGraphData): { nodes: LayoutNode[]; links: LayoutEdge[] } {
  return { nodes: data.nodes.map(n => ({ ...n })), links: data.edges.map(e => ({ ...e })) }
}
