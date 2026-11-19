import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, ChevronRight, CircleAlert, Database, ServerCog, ShieldCheck, XCircle } from 'lucide-react'
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
  const { data: diagnostics, refetch, isFetching, isError } = useQuery({
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
        description={summary && (
          <span className="inline-flex flex-wrap items-center gap-x-4 gap-y-1">
            <SummaryStat icon={<CheckCircle2 className="h-4 w-4 text-[var(--success)]" />} label="正常" value={summary.ok} />
            <SummaryStat icon={<CircleAlert className="h-4 w-4 text-[var(--warning)]" />} label="警告" value={summary.warning} />
            <SummaryStat icon={<XCircle className="h-4 w-4 text-[var(--danger)]" />} label="错误" value={summary.error} />
          </span>
        )}
        actions={
          <Btn variant="secondary" loading={isFetching} onClick={() => void refetch()}>
            刷新诊断
          </Btn>
        }
      >
        {isError && <p role="alert" className="text-sm text-[var(--danger)]">获取诊断结果失败，请刷新诊断。</p>}
        {!diagnostics && isFetching && <p role="status" className="text-sm text-soft">正在检查部署配置…</p>}
        {diagnostics && (
          <ul className="divide-y divide-[var(--border)] border-y border-[var(--border)]">
            {diagnostics.items.map((item) => <DiagnosticRow key={item.key} item={item} />)}
          </ul>
        )}
        <details className="group">
          <summary className="flex w-fit cursor-pointer list-none items-center gap-1.5 rounded-sm text-sm font-medium text-soft hover:text-[var(--text)] focus-visible:outline-[var(--accent)] [&::-webkit-details-marker]:hidden">
            <ChevronRight className="h-4 w-4 transition-transform group-open:rotate-90" />部署建议
          </summary>
          <ul className="mt-2 list-disc space-y-1 pl-6 text-sm leading-6 text-soft">
            <li>systemd 部署建议使用 <code className="wrap-anywhere">/etc/xray-pilot/ssh/id_ed25519</code> 作为默认私钥，并将 <code className="wrap-anywhere">/var/lib/xray-pilot/known_hosts</code> 设为服务用户可写。</li>
            <li>在「订阅与通知」填写公网 HTTPS 基址，作为订阅链接的前缀。</li>
            <li>配置异常时，结合上方诊断结果和操作日志排查。</li>
          </ul>
        </details>
      </Section>
    </>
  )
}

function InfoCard({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <SurfaceCard className="p-4">
      <div className="flex items-center gap-2 text-sm text-soft">
        <span className="text-[var(--accent)]">{icon}</span>
        {label}
      </div>
      <div className="mt-1 font-mono text-base">{value}</div>
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
    <li className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-x-2 py-3">
      <span className="mt-1" aria-hidden="true">{icon}</span>
      <div className="min-w-0 wrap-anywhere">
        <p className="leading-6">
          <span className="mr-2.5 inline-block text-[15px] font-semibold">{item.label}</span>
          <span className="text-sm text-soft">{item.detail}</span>
        </p>
        {item.value && <code className="mt-0.5 block font-mono text-[13px] leading-5 text-soft">{item.value}</code>}
        {item.suggestion && <p className="mt-1 text-sm leading-5 text-soft">{item.suggestion}</p>}
      </div>
      <Badge label={badgeLabel} variant={badgeVariant} />
    </li>
  )
}
