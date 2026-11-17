import { useEffect, useRef, useState } from 'react'

export function useChartWidth() {
  const container = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(600)
  useEffect(() => {
    const el = container.current
    if (!el) return
    const observer = new ResizeObserver(entries => setWidth(Math.max(220, entries[0].contentRect.width)))
    observer.observe(el)
    return () => observer.disconnect()
  }, [])
  return { container, width }
}
