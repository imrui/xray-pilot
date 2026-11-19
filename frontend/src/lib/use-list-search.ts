import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'

export function useListSearch() {
  const [params, setParams] = useSearchParams()
  const search = params.get('q') ?? ''
  const [keyword, setKeyword] = useState(search.trim())

  useEffect(() => {
    const timer = setTimeout(() => setKeyword(search.trim()), 300)
    return () => clearTimeout(timer)
  }, [search])

  const setSearch = (value: string) => {
    setParams(previous => {
      const next = new URLSearchParams(previous)
      if (value) next.set('q', value)
      else next.delete('q')
      return next
    }, { replace: true, preventScrollReset: true })
  }

  return { search, setSearch, keyword: search.trim() ? keyword : '' }
}
