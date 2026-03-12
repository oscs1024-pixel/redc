import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'
import { installHTTPTransport } from './lib/http-transport.js'

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
