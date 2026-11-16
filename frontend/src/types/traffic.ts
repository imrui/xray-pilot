export type TrafficDimension = 'all' | 'node' | 'user'
export type TrafficDirection = 'total' | 'up' | 'down'
export interface TrafficOption { id: number; name: string }
export interface TrafficAmount { up_bytes: number; down_bytes: number }
export interface TrafficPoint { time: string; up_bytes: number | null; down_bytes: number | null }
export interface TrafficSeries { id: string; name: string; points: TrafficPoint[] }
export interface TrafficAnalytics extends TrafficAmount {
  start: string; end: string; timezone: string; interval: 'hour' | 'day'
  active_users: number; active_nodes: number
  enabled_nodes: number; total_nodes: number; total_users: number; enabled_users: number; healthy_nodes: number
  successful_polls: number; failed_polls: number
  last_poll_at: string | null; retention_start: string | null; truncated: boolean
  nodes: TrafficOption[]; users: TrafficOption[]
  ranking: (TrafficOption & TrafficAmount)[]; series: TrafficSeries[]
}
export interface TrafficAnalyticsQuery {
  start: string; end: string; dimension: TrafficDimension; node_id?: number; user_id?: number
}
