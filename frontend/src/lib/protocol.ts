// 协议展示元数据（标签与徽章配色）的单一来源。
// 避免在节点列表、协议列表、节点协议抽屉等多处各自维护映射。
import type { Protocol } from '@/types'

export const PROTOCOL_LABELS: Record<Protocol, string> = {
  'vless-reality': 'VLESS + Reality',
  'vless-ws-tls': 'VLESS + WS + TLS',
  trojan: 'Trojan + TLS',
  hysteria2: 'Hysteria2',
}

export const PROTOCOL_BADGE_VARIANT: Record<Protocol, 'green' | 'blue' | 'yellow' | 'red'> = {
  'vless-reality': 'green',
  'vless-ws-tls': 'blue',
  trojan: 'yellow',
  hysteria2: 'red',
}

export function protocolLabel(protocol: string) {
  return PROTOCOL_LABELS[protocol as Protocol] ?? protocol
}

export function protocolBadgeVariant(protocol: string): 'green' | 'blue' | 'yellow' | 'red' | 'gray' {
  return PROTOCOL_BADGE_VARIANT[protocol as Protocol] ?? 'gray'
}

// 协议选项列表（新增/编辑表单下拉用），顺序与 PROTOCOL_LABELS 一致
export const PROTOCOL_OPTIONS = (Object.keys(PROTOCOL_LABELS) as Protocol[]).map((value) => ({
  value,
  label: PROTOCOL_LABELS[value],
}))
