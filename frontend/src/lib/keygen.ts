// 客户端侧 Reality short_id 生成（前端预览/草稿用，真正的密钥仍由后端生成）
export function generateShortIds(count = 6) {
  return Array.from({ length: count }, () => {
    const bytes = new Uint8Array(4)
    crypto.getRandomValues(bytes)
    return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
  })
}
