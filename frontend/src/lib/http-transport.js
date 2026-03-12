/**
 * HTTP Transport Polyfill for RedC GUI
 *
 * When running in HTTP Server mode (not inside Wails desktop app),
 * this module installs shims for:
 *   - window['go']['main']['App'] → POST /api/call
 *   - window.runtime → SSE-based event system
 *
 * The polyfill is transparent: all existing wailsjs code works unchanged.
 */

let sseSource = null;
const sseListeners = {};  // { eventName: [{ cb, remaining }] }

function getToken() {
  return localStorage.getItem('redc_token') || '';
}

function buildHeaders() {
  const t = getToken();
  return t ? { 'Authorization': `Bearer ${t}`, 'Content-Type': 'application/json' } : { 'Content-Type': 'application/json' };
}

async function apiCall(method, args) {
  const resp = await fetch('/api/call', {
    method: 'POST',
    headers: buildHeaders(),
    body: JSON.stringify({ method, args }),
  });
  if (resp.status === 401) {
    // Token invalid — prompt for new token
    promptToken();
    throw new Error('Unauthorized');
  }
  const data = await resp.json();
  if (data.error) throw new Error(data.error);
  return data.result;
}

function connectSSE() {
  if (sseSource) return;
  const token = getToken();
  const url = token ? `/api/events?token=${token}` : '/api/events';
  sseSource = new EventSource(url);

  sseSource.onmessage = (e) => {
    try {
      const msg = JSON.parse(e.data);
      const { event, data } = msg;
      if (!event || event === 'connected') return;
      const listeners = sseListeners[event] || [];
      const remaining = [];
      for (const entry of listeners) {
        entry.cb(data);
        if (entry.remaining === -1 || --entry.remaining > 0) {
          remaining.push(entry);
        }
      }
      sseListeners[event] = remaining;
    } catch (_) {}
  };

  sseSource.onerror = () => {
    sseSource = null;
    // Reconnect after 3s
    setTimeout(connectSSE, 3000);
  };
}

function subscribeEvent(name, cb, maxCallbacks) {
  if (!sseListeners[name]) sseListeners[name] = [];
  sseListeners[name].push({ cb, remaining: maxCallbacks });
  connectSSE();
  // Return unsubscribe function
  return () => {
    if (sseListeners[name]) {
      sseListeners[name] = sseListeners[name].filter(e => e.cb !== cb);
    }
  };
}

function promptToken() {
  const existing = localStorage.getItem('redc_token');
  const t = window.prompt(
    'RedC HTTP Server — 请输入访问 Token\n(Enter your access token)',
    existing || ''
  );
  if (t !== null && t.trim()) {
    localStorage.setItem('redc_token', t.trim());
    // Reconnect SSE with new token
    if (sseSource) {
      sseSource.close();
      sseSource = null;
    }
    connectSSE();
  }
}

export function installHTTPTransport() {
  // Install window['go']['main']['App'] proxy
  if (!window['go']) window['go'] = {};
  if (!window['go']['main']) window['go']['main'] = {};
  window['go']['main']['App'] = new Proxy({}, {
    get(_, method) {
      return (...args) => apiCall(method, args);
    }
  });

  // Install window.runtime shim
  window.runtime = {
    EventsOnMultiple(name, cb, maxCallbacks) {
      return subscribeEvent(name, cb, maxCallbacks);
    },
    EventsOn(name, cb) {
      return subscribeEvent(name, cb, -1);
    },
    EventsOnce(name, cb) {
      return subscribeEvent(name, cb, 1);
    },
    EventsOff(...names) {
      for (const name of names) {
        delete sseListeners[name];
      }
    },
    EventsEmit() {
      // No-op from frontend in HTTP mode (backend handles broadcasts)
    },
    // Window controls — no-op in browser
    WindowMinimise() {},
    WindowMaximise() {},
    WindowUnmaximise() {},
    WindowIsMaximised() { return Promise.resolve(false); },
    WindowToggleMaximise() {},
    WindowSetSize() {},
    WindowSetPosition() {},
    Quit() { window.close(); },
    // Environment — report as "web" platform
    Environment() {
      return Promise.resolve({ platform: 'web', arch: 'unknown', buildType: 'production' });
    },
    // Logging no-ops
    LogPrint() {}, LogTrace() {}, LogDebug() {}, LogInfo() {}, LogWarning() {}, LogError() {}, LogFatal() {},
  };

  // Check token on first load
  if (!getToken()) {
    promptToken();
  } else {
    connectSSE();
  }
}

export function isHTTPMode() {
  return !window.__wails_loaded__;
}
