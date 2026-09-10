// 客户端侧 Reality short_id 生成（前端预览/草稿用，真正的密钥仍由后端生成）
export function generateShortIds(count = 6) {
  return Array.from({ length: count }, () => {
    const bytes = new Uint8Array(4)
    crypto.getRandomValues(bytes)
    return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
  })
}

// 随机密码：去掉易混淆字符（0/O、1/l/I），长度默认 16，熵来自 crypto.getRandomValues
const PASSWORD_ALPHABET = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789'
export function generatePassword(length = 16) {
  const bytes = new Uint32Array(length)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (n) => PASSWORD_ALPHABET[n % PASSWORD_ALPHABET.length]).join('')
}
