import type { PageResult } from '@/types'

export async function fetchAllPages<T>(
  loadPage: (page: number, pageSize: number) => Promise<PageResult<T>>,
): Promise<T[]> {
  const items: T[] = []
  for (let page = 1; ; page++) {
    const { list, total } = await loadPage(page, 100)
    items.push(...list)
    if (!list.length || items.length >= total) return items
  }
}
