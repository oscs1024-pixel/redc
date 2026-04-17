import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { installHTTPTransport } from './lib/http-transport.js'

// Suppress benign ResizeObserver loop warning (xterm.js / layout-dependent observers)
const _origError = window.onerror
window.onerror = (msg, ...args) => {
  if (typeof msg === 'string' && msg.includes('ResizeObserver loop')) return true
  return _origError ? _origError(msg, ...args) : false
}

// Detect if running inside Wails desktop app
// Wails sets window.runtime synchronously before the app loads
if (!window.runtime) {
  // Running in HTTP server mode (browser) — install transport polyfill
  installHTTPTransport()
  window.__redcWebMode__ = true
}

const app = mount(App, {
  target: document.getElementById('app')
})

export default app
