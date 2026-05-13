import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// formatBytes 将字节数转换为人类可读字符串（二进制单位：KiB / MiB / GiB / TiB）
// 0 字节返回 "0 B"；负数返回 "-"（防御性处理）
export function formatBytes(bytes?: number | null): string {
  if (bytes === undefined || bytes === null) return '-'
  if (bytes < 0) return '-'
  if (bytes === 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  let value = bytes
  let i = 0
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i++
  }
  // < 10 时保留两位小数，否则保留一位
  const precision = value < 10 && i > 0 ? 2 : i === 0 ? 0 : 1
  return `${value.toFixed(precision)} ${units[i]}`
}
