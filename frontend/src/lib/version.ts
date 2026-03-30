const rawVersion = (import.meta.env.VITE_APP_VERSION ?? 'dev').trim()

export const APP_VERSION = rawVersion === 'dev' ? 'dev' : rawVersion.startsWith('v') ? rawVersion : `v${rawVersion}`
