import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'
import fs from 'fs'
import { createHash } from 'crypto'

// favicon 链接追加内容哈希：浏览器对 favicon 单独缓存，强制刷新也不失效，
// 换图后必须换 URL 才会重新拉取。哈希按文件内容算，图标不变则 URL 不变。
function faviconCacheBust(): Plugin {
  return {
    name: 'favicon-cache-bust',
    transformIndexHtml(html) {
      const svg = fs.readFileSync(path.resolve(__dirname, 'public/favicon.svg'))
      const hash = createHash('sha256').update(svg).digest('hex').slice(0, 8)
      return html.replace('href="/favicon.svg"', `href="/favicon.svg?v=${hash}"`)
    },
  }
}

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    faviconCacheBust(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    host: true,
    port: 5173,
    proxy: {
      '/api': 'http://localhost:2026',
      '/sub': 'http://localhost:2026',
    },
  },
})
