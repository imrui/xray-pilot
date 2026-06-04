// copyText 把文本写入剪贴板，兼容非 secure context（HTTP / 局域网 IP）。
//
// 现代 API navigator.clipboard.writeText 仅在 https 或 localhost 下可用；
// 在 http://192.168.x.x 之类的内网地址访问面板时会静默 reject，
// 触发"按钮点了没反应"的体验问题。
//
// 优先用 navigator.clipboard，失败时回退到 textarea + document.execCommand('copy')，
// 保证在主流浏览器的非 secure 环境下也能正常复制。
export async function copyText(text: string): Promise<boolean> {
  if (!text) return false

  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 落到 fallback
    }
  }

  return fallbackCopy(text)
}

function fallbackCopy(text: string): boolean {
  if (typeof document === 'undefined') return false

  const ta = document.createElement('textarea')
  ta.value = text
  // 避免出现在视口里造成滚动闪动
  ta.style.cssText =
    'position:fixed;top:0;left:0;width:1px;height:1px;padding:0;border:0;outline:0;opacity:0;pointer-events:none'
  ta.setAttribute('readonly', '')
  document.body.appendChild(ta)

  let ok = false
  try {
    ta.focus()
    ta.select()
    ta.setSelectionRange(0, text.length)
    ok = document.execCommand('copy')
  } catch {
    ok = false
  } finally {
    document.body.removeChild(ta)
  }
  return ok
}
