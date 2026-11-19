import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import { JSDOM } from 'jsdom'

test('二维码脚本未加载时，中转和节点弹窗可打开并复制当前地址', async t => {
  const relay = 'http://relay.test/sub/user'
  const node = 'vless://node.test'
  const template = await readFile(new URL('../../internal/handler/templates/subscribe.html', import.meta.url), 'utf8')
  // 模拟二维码库不可用，保留页面原有的 DOM 和事件处理器。
  const html = template.replace('{{.QRCodeScript}}', '').replaceAll('{{.RelayURL}}', relay).replaceAll('{{.Link}}', node)
  const dom = new JSDOM(html, {
    url: 'https://panel.test/sub/user', runScripts: 'dangerously',
  })
  t.after(() => dom.window.close())
  const { document, navigator } = dom.window
  await new Promise(resolve => dom.window.addEventListener('load', resolve, { once: true }))
  const dialog = document.querySelector('dialog')
  dialog.showModal = () => { dialog.open = true }
  let copied
  Object.defineProperty(navigator, 'clipboard', { value: { writeText: async text => { copied = text } } })

  function checkFallback(expected) {
    assert.ok(dialog.open)
    assert.ok(document.querySelector('#modalQR').hidden)
    assert.ok(document.querySelector('#modalSave').hidden)
    assert.ok(!document.querySelector('#modalQRUnavailable').hidden)
    const address = document.querySelector('#modalURL')
    assert.ok(!address.hidden)
    assert.equal(address.textContent, expected)
    const copy = document.querySelector('#modalCopy')
    assert.ok(!copy.hidden)
    copy.click()
    assert.equal(copied, expected)
  }

  assert.ok(document.querySelector('#mainSave').hidden)
  document.querySelector('[data-relay]').click()
  for (const format of ['', 'json', 'clash']) {
    document.querySelector(`#relayFormats [data-format="${format || 'default'}"]`).click()
    checkFallback(relay + (format ? '?format=' + format : ''))
  }
  dialog.open = false
  document.querySelector('.node-list .qr-btn').click()
  assert.ok(document.querySelector('#relayFormats').hidden)
  checkFallback(node)
})
