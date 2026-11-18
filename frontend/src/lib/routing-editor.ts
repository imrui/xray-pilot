// 编辑器身份仅存在于内存中，不进入策略 JSON；调序、重命名和不可变更新均保留身份。
const itemKeys = new WeakMap<object, number>()
let nextItemKey = 0

export function editorItemKey(item: object): number {
  let key = itemKeys.get(item)
  if (key === undefined) {
    key = ++nextItemKey
    itemKeys.set(item, key)
  }
  return key
}

export function retainEditorItemKey<T extends object>(previous: object, next: T): T {
  itemKeys.set(next, editorItemKey(previous))
  return next
}

export function wireGuardReservedError(value: unknown): string {
  if (value === undefined) return ''
  return Array.isArray(value) && (value.length === 0 || value.length === 3) &&
    value.every((v) => typeof v === 'number' && Number.isInteger(v) && v >= 0 && v <= 255)
    ? ''
    : 'Reserved 必须是 3 个 0–255 的整数，用英文逗号分隔。'
}
