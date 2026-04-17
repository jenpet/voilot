/**
 * Shared WebSocket connection to the voilot backend.
 * Manages connection lifecycle, reconnection with exponential backoff,
 * and message routing. HMR-safe: uses window-level state to survive
 * module hot-reloads without orphaning connections.
 */

import { useDebugLog } from './useDebugLog';

// Lazy debug log accessor for module-level functions
function _log(level: 'debug' | 'info' | 'warn' | 'error', event: string, data?: Record<string, unknown>) {
  try {
    const { log } = useDebugLog();
    log(level, 'ws', event, data);
  } catch {
    // useDebugLog may not be available during SSR or early init
  }
}

export interface AgentEvent {
  type: 'text' | 'code' | 'tool_use' | 'tool_result' | 'thinking' | 'error' | 'done' | 'status' | 'session_created' | 'session_updated' | 'permission_request' | 'permission_replied' | 'question_request' | 'question_replied'
  sessionId?: string
  messageId?: string
  partId?: string
  content: string
  language?: string
  delta?: string
  meta?: Record<string, unknown>
}

export interface ChatOutbound {
  type: 'event' | 'command' | 'error'
  sessionId?: string
  event?: AgentEvent
  content?: string
  meta?: Record<string, unknown>
}

type MessageHandler = (msg: ChatOutbound) => void

// Reconnect backoff configuration
const RECONNECT_BASE_MS = 1000
const RECONNECT_MAX_MS = 30000
const RECONNECT_MULTIPLIER = 1.5

// HMR-safe global state: store on window to survive module hot-reloads
interface WsGlobalState {
  ws: WebSocket | null
  reconnectTimer: ReturnType<typeof setTimeout> | null
  reconnectAttempt: number
  handlers: Set<MessageHandler>
  connectionState: Ref<'connecting' | 'connected' | 'disconnected'>
  initialized: boolean
}

function getGlobalState(): WsGlobalState {
  if (typeof window === 'undefined') {
    // SSR fallback — never actually used since ssr: false, but keeps types safe
    return {
      ws: null,
      reconnectTimer: null,
      reconnectAttempt: 0,
      handlers: new Set(),
      connectionState: ref('disconnected'),
      initialized: false,
    }
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const win = window as any
  if (!win.__voilot_ws) {
    win.__voilot_ws = {
      ws: null,
      reconnectTimer: null,
      reconnectAttempt: 0,
      handlers: new Set<MessageHandler>(),
      connectionState: ref<'connecting' | 'connected' | 'disconnected'>('disconnected'),
      initialized: false,
    }
  }
  return win.__voilot_ws
}

function getWsUrl(): string {
  if (typeof window === 'undefined') return ''
  const backendUrl = resolveBackendUrl()

  if (backendUrl) {
    // Connect directly to backend (dev mode or remote device access)
    const wsUrl = backendUrl.replace(/^http/, 'ws')
    return `${wsUrl}/ws/chat`
  }

  // Fallback: build from current location (works behind Nginx in production)
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  return `${protocol}//${host}/ws/chat`
}

function scheduleReconnect(state: WsGlobalState) {
  if (state.reconnectTimer) {
    clearTimeout(state.reconnectTimer)
  }
  const delay = Math.min(
    RECONNECT_BASE_MS * Math.pow(RECONNECT_MULTIPLIER, state.reconnectAttempt),
    RECONNECT_MAX_MS,
  )
  state.reconnectAttempt++
  _log('info', 'reconnect_scheduled', { delayMs: Math.round(delay), attempt: state.reconnectAttempt })
  console.log(`[ws] Reconnecting in ${Math.round(delay)}ms (attempt ${state.reconnectAttempt})...`)
  state.reconnectTimer = setTimeout(() => connectInternal(state), delay)
}

function connectInternal(state: WsGlobalState) {
  // Guard: don't create duplicate connections
  if (state.ws && (state.ws.readyState === WebSocket.OPEN || state.ws.readyState === WebSocket.CONNECTING)) {
    return
  }

  const url = getWsUrl()
  if (!url) return

  state.connectionState.value = 'connecting'
  _log('info', 'connecting', { url })

  try {
    state.ws = new WebSocket(url)
  } catch {
    console.warn('[ws] Failed to create WebSocket, scheduling reconnect')
    _log('error', 'creation_failed')
    scheduleReconnect(state)
    return
  }

  state.ws.onopen = () => {
    state.connectionState.value = 'connected'
    state.reconnectAttempt = 0 // Reset backoff on successful connection
    _log('info', 'connected')
    console.log('[ws] Connected to voilot backend')
    if (state.reconnectTimer) {
      clearTimeout(state.reconnectTimer)
      state.reconnectTimer = null
    }
  }

  state.ws.onmessage = (event) => {
    try {
      const msg: ChatOutbound = JSON.parse(event.data)
      state.handlers.forEach(handler => handler(msg))
    } catch {
      console.warn('[ws] Failed to parse message:', event.data)
    }
  }

  state.ws.onclose = (event) => {
    state.connectionState.value = 'disconnected'
    _log('warn', 'disconnected', { code: event.code, reason: event.reason, wasClean: event.wasClean })
    state.ws = null
    if (!event.wasClean) {
      scheduleReconnect(state)
    }
  }

  state.ws.onerror = () => {
    // onclose will fire after this, triggering reconnect via scheduleReconnect
  }
}

function disconnectInternal(state: WsGlobalState) {
  if (state.reconnectTimer) {
    clearTimeout(state.reconnectTimer)
    state.reconnectTimer = null
  }
  state.reconnectAttempt = 0
  if (state.ws) {
    state.ws.close(1000, 'Client disconnect')
    state.ws = null
  }
  state.connectionState.value = 'disconnected'
}

export function useWebSocket() {
  const state = getGlobalState()
  const { log } = useDebugLog()

  // Auto-connect on first use (only once, survives HMR)
  if (!state.initialized && typeof window !== 'undefined') {
    state.initialized = true
    connectInternal(state)
  }

  function connect() {
    state.reconnectAttempt = 0
    connectInternal(state)
  }

  function disconnect() {
    disconnectInternal(state)
  }

  function send(msg: { type: string; sessionId?: string; [key: string]: unknown }): boolean {
    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
      log('warn', 'ws', 'send_failed_not_connected', { msgType: msg.type })
      console.warn('[ws] Cannot send, not connected')
      return false
    }
    log('debug', 'ws', 'message_sent', { msgType: msg.type, sessionId: msg.sessionId })
    state.ws.send(JSON.stringify(msg))
    return true
  }

  function subscribe(handler: MessageHandler): () => void {
    state.handlers.add(handler)
    return () => {
      state.handlers.delete(handler)
    }
  }

  return {
    connectionState: readonly(state.connectionState),
    connect,
    disconnect,
    send,
    subscribe,
  }
}
