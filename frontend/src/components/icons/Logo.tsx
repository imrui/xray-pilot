interface LogoProps {
  size?: number
  className?: string
}

// 节点拓扑 X：四个节点经中心枢纽连成 X。
// 颜色走 currentColor，由调用方传 text-[var(--accent)]，明暗主题自动切换；中心枢纽为空心环，底色随所在表面。
// 同一图形另有两份：public/favicon.svg（小尺寸加粗版）与 handler/templates/subscribe.html（订阅页，绿色主色），改形状时三处同步。
export function Logo({ size = 32, className }: LogoProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-hidden="true"
    >
      <path
        d="M7.5 7.5L13 13M24.5 24.5L19 19M24.5 7.5L19 13M7.5 24.5L13 19"
        stroke="currentColor"
        strokeWidth="2.6"
        strokeLinecap="round"
      />
      <circle cx="7.5" cy="7.5" r="3.2" fill="currentColor" />
      <circle cx="24.5" cy="7.5" r="3.2" fill="currentColor" />
      <circle cx="7.5" cy="24.5" r="3.2" fill="currentColor" />
      <circle cx="24.5" cy="24.5" r="3.2" fill="currentColor" />
      <circle cx="16" cy="16" r="3.6" stroke="currentColor" strokeWidth="2.6" />
    </svg>
  )
}
