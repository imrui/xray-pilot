import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, CircleAlert, Database, ServerCog, ShieldCheck, Wrench, XCircle } from 'lucide-react'
import { systemApi } from '@/lib/api'
import { Btn } from '@/components/ui/Form'
import { SurfaceCard } from '@/components/ui/Page'
import { Badge } from '@/components/ui/Badge'
import type { DiagnosticItem } from '@/types'
import { Section } from './shared'

// 运行状态：只读信息 + 部署诊断 + 部署建议，不参与保存流程。
// 布局只有一层卡片：顶部三项各自是独立卡片，诊断项在 Section 内用分隔线列表，不再卡片套卡片。
export default function StatusSection() {
  const { data: info } = useQuery({
    queryKey: ['system-info'],
    queryFn: () => systemApi.getInfo().then((r) => r.data.data!),
  })
  const { data: diagnostics, refetch, isFetching } = useQuery({
    queryKey: ['system-diagnostics'],
    queryFn: () => systemApi.getDiagnostics().then((r) => r.data.data!),
  })

  const summary = diagnostics?.summary

  return (
    <>
      <div className="grid gap-4 md:grid-cols-3">
        <InfoCard icon={<ServerCog className="h-4 w-4" />} label="服务端口" value={String(info?.server.port ?? '—')} />
        <InfoCard icon={<ShieldCheck className="h-4 w-4" />} label="运行模式" value={String(info?.server.mode ?? '—')} />
        <InfoCard icon={<Database className="h-4 w-4" />} label="数据库" value={String(info?.database.driver ?? '—')} />
      </div>

      <Section
        title="部署诊断"
        description="确认服务路径、数据库目录、SSH 默认路径和订阅基址配置是否就绪。"
        actions={
          <Btn variant="secondary" loading={isFetching} onClick={() => void refetch()}>
            刷新诊断
          </Btn>
        }
      >
        <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-sm">
          <SummaryStat icon={<CheckCircle2 className="h-4 w-4 text-[var(--success)]" />} label="正常" value={summary?.ok ?? 0} />
          <SummaryStat icon={<CircleAlert className="h-4 w-4 text-[var(--warning)]" />} label="警告" value={summary?.warning ?? 0} />
          <SummaryStat icon={<XCircle className="h-4 w-4 text-[var(--danger)]" />} label="错误" value={summary?.error ?? 0} />
        </div>
        <ul className="divide-y divide-[var(--border)] border-t border-[var(--border)]">
          {diagnostics?.items.map((item) => <DiagnosticRow key={item.key} item={item} />)}
        </ul>
      </Section>

      <SurfaceCard className="p-5">
        <div className="flex items-center gap-2 text-sm font-semibold">
          <Wrench className="h-4 w-4 text-[var(--accent)]" />
          部署建议
        </div>
        <div className="mt-4 space-y-3 text-sm leading-6 text-soft">
          <p>systemd 部署建议使用 `/etc/xray-pilot/ssh/id_ed25519` 作为默认私钥路径，并在「订阅与通知」显式配置公网 HTTPS 域名作为订阅链接前缀。</p>
          <p>`known_hosts` 建议保持在 `/var/lib/xray-pilot/known_hosts`，方便与运行用户权限对齐。</p>
          <p>配置保存后若未达预期，先看上方诊断项，再结合操作日志排查。</p>
        </div>
      </SurfaceCard>
    </>
  )
}

function InfoCard({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <SurfaceCard className="p-4">
      <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">
        <span className="text-[var(--accent)]">{icon}</span>
        {label}
      </div>
      <div className="mt-2 font-mono text-sm">{value}</div>
    </SurfaceCard>
  )
}

function SummaryStat({ icon, label, value }: { icon: React.ReactNode; label: string; value: number }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-soft">
      {icon}
      {label}
      <span className="font-semibold text-[var(--text)]">{value}</span>
    </span>
  )
}

function DiagnosticRow({ item }: { item: DiagnosticItem }) {
  const icon = item.status === 'ok'
    ? <CheckCircle2 className="h-4 w-4 text-[var(--success)]" />
    : item.status === 'warning'
      ? <CircleAlert className="h-4 w-4 text-[var(--warning)]" />
      : <XCircle className="h-4 w-4 text-[var(--danger)]" />
  const badgeVariant = item.status === 'ok' ? 'green' : item.status === 'warning' ? 'yellow' : 'red'
  const badgeLabel = item.status === 'ok' ? '正常' : item.status === 'warning' ? '警告' : '错误'

  return (
    <li className="flex flex-col gap-2 py-4 md:flex-row md:items-start md:justify-between md:gap-4">
      <div className="min-w-0 space-y-1.5">
        <div className="flex items-center gap-2">
          {icon}
          <div className="text-sm font-semibold">{item.label}</div>
        </div>
        {item.value && <div className="break-all font-mono text-xs text-soft">{item.value}</div>}
        <p className="text-sm leading-6 text-soft">{item.detail}</p>
        {item.suggestion && <p className="text-xs leading-5 text-faint">建议：{item.suggestion}</p>}
      </div>
      <Badge label={badgeLabel} variant={badgeVariant} />
    </li>
  )
}
