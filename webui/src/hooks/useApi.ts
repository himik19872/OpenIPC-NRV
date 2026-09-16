import { useState, useEffect, useCallback } from 'react'
import { AxiosResponse } from 'axios'

export function useAsync<T>(
  fn: () => Promise<AxiosResponse<T>>,
  deps: any[] = []
) {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const execute = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fn()
      setData(res.data)
    } catch (err: any) {
      setError(err.response?.data?.error || err.message)
    } finally {
      setLoading(false)
    }
  }, deps)

  useEffect(() => { execute() }, [execute])

  return { data, loading, error, refetch: execute }
}

export function useAuth() {
  const [token, setToken] = useState<string | null>(localStorage.getItem('token'))

  const login = (t: string) => {
    localStorage.setItem('token', t)
    setToken(t)
  }

  const logout = () => {
    localStorage.removeItem('token')
    setToken(null)
    window.location.href = '/login'
  }

  return { token, isAuthenticated: !!token, login, logout }
}