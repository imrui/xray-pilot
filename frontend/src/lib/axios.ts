import axios, { AxiosError } from 'axios'
import type { ApiResponse } from '@/types'
import { pushToast } from '@/lib/notify'

const request = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

// 请求拦截器：自动附加 JWT Token
request.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 响应拦截器：401 跳转登录
request.interceptors.response.use(
  (response) => {
    const payload = response.data as ApiResponse<unknown> | undefined
    if (payload && typeof payload === 'object' && 'code' in payload && payload.code !== 0) {
      const message = payload.message || '请求失败'
      pushToast({
        title: '接口请求失败',
        description: message,
        variant: 'error',
      })
      return Promise.reject(new Error(message))
    }
    return response
  },
  (error) => {
    if (error.response?.status === 401) {
      // 与 store/auth.logout 同步清理（token / 用户名 / 角色）
      localStorage.removeItem('token')
      localStorage.removeItem('admin_username')
      localStorage.removeItem('admin_role')
      window.location.href = '/login'
      return Promise.reject(error)
    }

    const axiosError = error as AxiosError<ApiResponse<unknown>>
    const message =
      axiosError.response?.data?.message ||
      axiosError.message ||
      '网络异常，请稍后重试'

    pushToast({
      title: axiosError.response ? '接口请求失败' : '网络连接异常',
      description: message,
      variant: 'error',
    })

    return Promise.reject(new Error(message))
  }
)

export default request
