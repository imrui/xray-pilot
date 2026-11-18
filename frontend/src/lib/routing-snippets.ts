import type { CustomOutbound, RoutingRuleItem } from './routing-model'

export interface OutboundSnippet {
  key: string
  name: string
  description: string
  build: (existingTags: string[]) => CustomOutbound
}

export interface RuleSnippet {
  key: string
  name: string
  description: string
  fixedTarget?: string
  defaultTarget: (outbounds: CustomOutbound[]) => string
  build: (targetOutboundTag: string) => RoutingRuleItem[]
}

function uniqueTag(baseTag: string, existingTags: string[]): string {
  const used = new Set(existingTags)
  if (!used.has(baseTag)) return baseTag
  let suffix = 2
  while (used.has(`${baseTag}-${suffix}`)) {
    suffix += 1
  }
  return `${baseTag}-${suffix}`
}

export const OUTBOUND_SNIPPETS: readonly OutboundSnippet[] = [
  {
    key: 'warp-wireguard',
    name: 'Cloudflare WARP (WireGuard)',
    description: '预填官方 Endpoint 与公钥，只需补充本机分配的 secretKey 与隧道双栈内网地址。',
    build: (existingTags) => ({
      tag: uniqueTag('warp-egress', existingTags),
      protocol: 'wireguard',
      remark: 'Cloudflare WARP 解锁出口',
      settings: {
        secretKey: '',
        address: ['172.16.0.2/32', '2606:4700:110::2/128'],
        peers: [
          {
            endpoint: 'engage.cloudflareclient.com:2408',
            publicKey: 'bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=',
            keepAlive: 25,
          },
        ],
        mtu: 1280,
      },
    }),
  },
  {
    key: 'freedom-sendthrough',
    name: '多 IP 服务器指定出口 (Freedom sendThrough)',
    description: '在拥有多个公网 IP 的机器上指定特定出口源 IP 发起连接。',
    build: (existingTags) => ({
      tag: uniqueTag('secondary-ip', existingTags),
      protocol: 'freedom',
      remark: '指定副 IP 出口',
      sendThrough: '',
      settings: {
        domainStrategy: 'AsIs',
      },
    }),
  },
  {
    key: 'socks5-dialer-proxy',
    name: '前置链式代理落地 (SOCKS5 + dialerProxy)',
    description: '经由强制 IPv4 直连拨号至外部 SOCKS5 落地机，适合单栈转发与流媒体分流。',
    build: (existingTags) => ({
      tag: uniqueTag('landing-socks', existingTags),
      protocol: 'socks',
      remark: '外部 SOCKS5 链式落地',
      settings: {
        servers: [
          {
            address: '',
            port: 1080,
          },
        ],
      },
      streamSettings: {
        sockopt: {
          dialerProxy: 'direct-v4',
        },
      },
    }),
  },
]

export const RULE_SNIPPETS: readonly RuleSnippet[] = [
  {
    key: 'block-bt',
    name: '屏蔽 BT 协议下载',
    description: '拦截嗅探到的 BitTorrent P2P 流量，避免 VPS 触发机房版权投诉。',
    defaultTarget: () => 'block',
    build: (targetOutboundTag) => [
      {
        type: 'field',
        remark: '屏蔽 BT 流量',
        protocol: ['bittorrent'],
        outboundTag: targetOutboundTag,
      },
    ],
  },
  {
    key: 'block-ads',
    name: '屏蔽常见广告域名',
    description: '基于 geosite:category-ads-all 拦截常见广告与追踪器请求。',
    defaultTarget: () => 'block',
    build: (targetOutboundTag) => [
      {
        type: 'field',
        remark: '屏蔽常见广告域名',
        domain: ['geosite:category-ads-all'],
        outboundTag: targetOutboundTag,
      },
    ],
  },
  {
    key: 'ai-services',
    name: '主流 AI 服务集合',
    description: '包含 geosite:openai、geosite:anthropic 与 geosite:google-gemini。',
    defaultTarget: (outbounds) => outbounds[0]?.tag || 'direct-v4',
    build: (targetOutboundTag) => [
      {
        type: 'field',
        remark: '主流 AI 服务分流',
        domain: ['geosite:openai', 'geosite:anthropic', 'geosite:google-gemini'],
        outboundTag: targetOutboundTag,
      },
    ],
  },
  {
    key: 'streaming-media',
    name: '主流流媒体集合',
    description: '包含 geosite:netflix、geosite:disney 与 geosite:youtube。',
    defaultTarget: (outbounds) => outbounds[0]?.tag || 'direct-v6',
    build: (targetOutboundTag) => [
      {
        type: 'field',
        remark: '流媒体集合分流',
        domain: ['geosite:netflix', 'geosite:disney', 'geosite:youtube'],
        outboundTag: targetOutboundTag,
      },
    ],
  },
  {
    key: 'telegram-suite',
    name: 'Telegram 域名与 IP 集合',
    description: '同时插入 geosite:telegram 域名规则与 geoip:telegram 目标 IP 规则。',
    defaultTarget: (outbounds) => outbounds[0]?.tag || 'direct-v4',
    build: (targetOutboundTag) => [
      {
        type: 'field',
        remark: 'Telegram 域名分流',
        domain: ['geosite:telegram'],
        outboundTag: targetOutboundTag,
      },
      {
        type: 'field',
        remark: 'Telegram IP 分流',
        ip: ['geoip:telegram'],
        outboundTag: targetOutboundTag,
      },
    ],
  },
]
