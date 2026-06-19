import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import Sidebar from '../components/Sidebar'
import { useWebSocket, ServerMsg, Op } from '../hooks/useWebSocket'
import { api, Metrics } from '../services/api'

interface PresenceUser { id: string; name: string }

const COLORS = ['#e74c3c', '#3498db', '#9b59b6', '#f39c12', '#1abc9c', '#e67e22']

function avatarColor(userId: string) {
  let hash = 0
  for (const c of userId) hash = (hash * 31 + c.charCodeAt(0)) >>> 0
  return COLORS[hash % COLORS.length]
}

export default function EditorPage() {
  const { id: docId } = useParams<{ id: string }>()
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const contentRef = useRef('')
  const [content, setContent] = useState('')
  const [docTitle, setDocTitle] = useState('Carregando...')
  const [users, setUsers] = useState<PresenceUser[]>([])
  const [metrics, setMetrics] = useState<Metrics | null>(null)
  const myId = localStorage.getItem('userId') ?? ''

  // Keep contentRef in sync for use inside WS handler (avoids stale closures)
  useEffect(() => { contentRef.current = content }, [content])

  // Load document title
  useEffect(() => {
    if (!docId) return
    api.listDocuments().then(docs => {
      const found = docs.find(d => d.id === docId)
      if (found) setDocTitle(found.title)
    })
  }, [docId])

  // Metrics polling (every 10s)
  useEffect(() => {
    if (!docId) return
    const load = () => api.getMetrics(docId).then(setMetrics).catch(() => {})
    load()
    const t = setInterval(load, 10_000)
    return () => clearInterval(t)
  }, [docId])

  const handleMessage = useCallback((msg: ServerMsg) => {
    switch (msg.type) {
      case 'resync':
        setContent(msg.content)
        break

      case 'op': {
        const ta = textareaRef.current
        const savedStart = ta?.selectionStart ?? 0
        const savedEnd = ta?.selectionEnd ?? 0

        setContent(prev => {
          const next = applyOp(prev, msg.op)
          // Adjust cursor position to account for the remote op
          if (ta) {
            const newStart = adjustCursor(savedStart, msg.op)
            const newEnd = adjustCursor(savedEnd, msg.op)
            requestAnimationFrame(() => {
              ta.selectionStart = newStart
              ta.selectionEnd = newEnd
            })
          }
          return next
        })
        break
      }

      case 'presence':
        setUsers(msg.users.filter(u => u.id !== myId))
        break
    }
  }, [myId])

  const { connected, sendOp } = useWebSocket(docId!, { onMessage: handleMessage })

  function handleChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    const newVal = e.target.value
    const old = contentRef.current
    const ops = diffToOps(old, newVal)
    setContent(newVal)
    ops.forEach(op => sendOp(op))
  }

  return (
    <div className="app-layout">
      <Sidebar />

      <div className="editor-area">
        {/* Top bar: connection status + presence + metrics */}
        <div className="editor-topbar">
          {metrics && (
            <div className="metrics-badge">
              <span title="Total de operações">⚡ {metrics.totalOps.toLocaleString()} ops</span>
              <span title="Caracteres inseridos">+{metrics.charsInserted.toLocaleString()}</span>
              <span title="Caracteres removidos">−{metrics.charsDeleted.toLocaleString()}</span>
            </div>
          )}

          <div className="presence-list">
            {users.map(u => (
              <div
                key={u.id}
                className="presence-avatar"
                style={{ background: avatarColor(u.id) }}
                title={u.name}
              >
                {u.name[0]?.toUpperCase()}
              </div>
            ))}
          </div>

          <div
            className="editor-conn-dot"
            style={{ background: connected ? '#22c55e' : '#ef4444' }}
          />
          <span className="editor-conn-label">
            {connected ? 'ao vivo' : 'reconectando...'}
          </span>
        </div>

        {/* Editor scroll area */}
        <div className="editor-scroll">
          <div className="editor-page">
            <h1
              className="editor-title"
              style={{ cursor: 'default' }}
            >
              {docTitle}
            </h1>

            <div className="editor-divider" />

            <textarea
              ref={textareaRef}
              className="editor-body"
              value={content}
              onChange={handleChange}
              placeholder="Pressione Enter e comece a escrever…"
              autoFocus
            />
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Helpers ───────────────────────────────────────────────

function applyOp(content: string, op: Op): string {
  const chars = [...content]
  const pos = Math.max(0, Math.min(op.pos, chars.length))
  if (op.type === 'insert' && op.char) {
    chars.splice(pos, 0, op.char)
  } else if (op.type === 'delete') {
    if (pos < chars.length) chars.splice(pos, 1)
  }
  return chars.join('')
}

function adjustCursor(cursor: number, op: Op): number {
  if (op.type === 'insert' && op.pos <= cursor) return cursor + 1
  if (op.type === 'delete' && op.pos < cursor) return cursor - 1
  return cursor
}

// Compute the minimal set of ops to go from oldStr to newStr.
// Handles typing (1 char), paste (N chars) and deletion (1 or more chars).
function diffToOps(oldStr: string, newStr: string): Op[] {
  const ops: Op[] = []

  // Find common prefix
  let i = 0
  const minLen = Math.min(oldStr.length, newStr.length)
  while (i < minLen && oldStr[i] === newStr[i]) i++

  // Find common suffix (from end, not overlapping prefix)
  let oi = oldStr.length - 1
  let ni = newStr.length - 1
  while (oi >= i && ni >= i && oldStr[oi] === newStr[ni]) { oi--; ni-- }

  // Delete chars old[i..oi] — iterate in reverse so positions stay valid
  for (let k = oi; k >= i; k--) {
    ops.push({ type: 'delete', pos: k })
  }

  // Insert chars new[i..ni]
  for (let k = i; k <= ni; k++) {
    // After each insert the position advances by 1 for subsequent inserts
    ops.push({ type: 'insert', pos: k, char: newStr[k] })
  }

  return ops
}
