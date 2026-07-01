import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import Sidebar from '../components/Sidebar'
import { useWebSocket, ServerMsg, Op } from '../hooks/useWebSocket'
import { api, DocumentAnalytics } from '../services/api'

interface PresenceUser { id: string; name: string }

interface RemoteCursor { pos: number; name: string }

interface RemoteEdit { id: number; pos: number; userId: string }

const HIGHLIGHT_DURATION_MS = 700

const COLORS = ['#e74c3c', '#3498db', '#9b59b6', '#f39c12', '#1abc9c', '#e67e22']

function avatarColor(userId: string) {
  let hash = 0
  for (const c of userId) hash = (hash * 31 + c.charCodeAt(0)) >>> 0
  return COLORS[hash % COLORS.length]
}

export default function EditorPage() {
  const { id: docId } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const contentRef = useRef('')
  const [content, setContent] = useState('')
  const [docTitle, setDocTitle] = useState('Carregando...')
  const [users, setUsers] = useState<PresenceUser[]>([])
  const [analytics, setAnalytics] = useState<DocumentAnalytics | null>(null)
  const [cursors, setCursors] = useState<Map<string, RemoteCursor>>(new Map())
  const [highlights, setHighlights] = useState<RemoteEdit[]>([])
  const highlightIdRef = useRef(0)
  const myId = localStorage.getItem('userId') ?? ''

  useEffect(() => { contentRef.current = content }, [content])

  useEffect(() => {
    if (!docId) return

    let cancelled = false
    setContent('')
    contentRef.current = ''
    setDocTitle('Carregando...')
    setUsers([])
    setAnalytics(null)

    api.getDocument(docId)
      .then(doc => {
        if (!cancelled) setDocTitle(doc.title)
      })
      .catch(() => {
        if (!cancelled) navigate('/documents', { replace: true })
      })

    return () => {
      cancelled = true
    }
  }, [docId, navigate])

  useEffect(() => {
    if (!docId) return
    const load = () => api.getDocumentAnalytics(docId).then(setAnalytics).catch(() => {})
    load()
    const t = setInterval(load, 5_000)
    return () => clearInterval(t)
  }, [docId])

  const handleMessage = useCallback((msg: ServerMsg) => {
    switch (msg.type) {
      case 'resync':
        setContent(msg.content ?? '')
        setCursors(new Map())
        break

      case 'op': {
        if (!msg.op) { console.warn('[editor] op message missing op field', msg); break }
        const ta = textareaRef.current
        const savedStart = ta?.selectionStart ?? 0
        const savedEnd = ta?.selectionEnd ?? 0

        setContent(prev => {
          const next = applyOp(prev, msg.op!)
          if (ta) {
            const newStart = adjustCursor(savedStart, msg.op!)
            const newEnd = adjustCursor(savedEnd, msg.op!)
            requestAnimationFrame(() => {
              ta.selectionStart = newStart
              ta.selectionEnd = newEnd
            })
          }
          return next
        })

        // Adjust all remote cursor positions for the incoming op
        setCursors(prev => {
          const m = new Map(prev)
          for (const [id, cur] of m) {
            m.set(id, { ...cur, pos: adjustCursor(cur.pos, msg.op!) })
          }
          return m
        })

        // Keep existing highlight markers aligned, then flash a new one so
        // users can see who inserted what — attributed by color, fading out.
        setHighlights(prev => prev.map(h => ({ ...h, pos: adjustCursor(h.pos, msg.op!) })))
        if (msg.op.type === 'insert' && msg.userId && msg.userId !== myId) {
          const id = ++highlightIdRef.current
          setHighlights(prev => [...prev, { id, pos: msg.op!.pos, userId: msg.userId }])
          window.setTimeout(() => {
            setHighlights(prev => prev.filter(h => h.id !== id))
          }, HIGHLIGHT_DURATION_MS)
        }
        break
      }

      case 'cursor':
        setCursors(prev => {
          const m = new Map(prev)
          m.set(msg.userId, { pos: msg.pos, name: msg.name })
          return m
        })
        break

      case 'presence':
        setUsers((msg.users ?? []).filter(u => u.id !== myId))
        // Remove cursors for users who left
        setCursors(prev => {
          const active = new Set((msg.users ?? []).map(u => u.id))
          const m = new Map(prev)
          for (const id of m.keys()) {
            if (!active.has(id)) m.delete(id)
          }
          return m
        })
        break
    }
  }, [myId])

  const { connected, sendOp, sendCursor } = useWebSocket(docId!, { onMessage: handleMessage })

  function handleChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    const newVal = e.target.value
    const old = contentRef.current
    const ops = diffToOps(old, newVal)
    setContent(newVal)
    ops.forEach(op => sendOp(op))
    sendCursor(e.target.selectionStart)
  }

  function handleCursorMove(e: React.SyntheticEvent<HTMLTextAreaElement>) {
    sendCursor(e.currentTarget.selectionStart)
  }

  return (
    <div className="app-layout">
      <Sidebar />

      <div className="editor-area">
        <div className="editor-topbar">
          {analytics && (
            <div className="analytics-strip" title="Analytics do conteúdo persistido">
              <span className="analytics-strip-label">Analytics</span>
              <span>{analytics.charCount.toLocaleString()} chars</span>
              <span>{analytics.wordCount.toLocaleString()} palavras</span>
              <span>{analytics.lineCount.toLocaleString()} linhas</span>
              <span>{analytics.paragraphCount.toLocaleString()} parágrafos</span>
              <span>v{analytics.version.toLocaleString()}</span>
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

        <div className="editor-scroll">
          <div className="editor-page">
            <h1 className="editor-title" style={{ cursor: 'default' }}>
              {docTitle}
            </h1>

            <div className="editor-divider" />

            {/* Wrapper for cursor overlay */}
            <div style={{ position: 'relative' }}>
              <textarea
                ref={textareaRef}
                className="editor-body"
                value={content}
                onChange={handleChange}
                onClick={handleCursorMove}
                onKeyUp={handleCursorMove}
                placeholder="Pressione Enter e comece a escrever…"
                autoFocus
              />
              <RemoteEditOverlay
                taRef={textareaRef}
                highlights={highlights}
                content={content}
              />
              <CursorOverlay
                taRef={textareaRef}
                cursors={cursors}
                content={content}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Cursor overlay ─────────────────────────────────────────────────────────────

interface CursorOverlayProps {
  taRef: React.RefObject<HTMLTextAreaElement>
  cursors: Map<string, RemoteCursor>
  content: string
}

interface ComputedCursor {
  id: string
  top: number
  left: number
  name: string
  color: string
}

function CursorOverlay({ taRef, cursors, content }: CursorOverlayProps) {
  const [positions, setPositions] = useState<ComputedCursor[]>([])

  useLayoutEffect(() => {
    const ta = taRef.current
    if (!ta || cursors.size === 0) { setPositions([]); return }

    const computed = [...cursors.entries()].flatMap(([id, cursor]) => {
      try {
        const coords = getCaretCoords(ta, cursor.pos)
        return [{ id, ...coords, name: cursor.name, color: avatarColor(id) }]
      } catch {
        return []
      }
    })
    setPositions(computed)
  }, [cursors, content, taRef])

  // Recompute on scroll so cursors track the visible area
  useEffect(() => {
    const ta = taRef.current
    if (!ta) return
    const onScroll = () => {
      const computed = [...cursors.entries()].flatMap(([id, cursor]) => {
        try {
          const coords = getCaretCoords(ta, cursor.pos)
          return [{ id, ...coords, name: cursor.name, color: avatarColor(id) }]
        } catch {
          return []
        }
      })
      setPositions(computed)
    }
    ta.addEventListener('scroll', onScroll)
    return () => ta.removeEventListener('scroll', onScroll)
  }, [cursors, taRef])

  if (positions.length === 0) return null

  return (
    <div style={{
      position: 'absolute',
      inset: 0,
      pointerEvents: 'none',
      overflow: 'hidden',
    }}>
      {positions.map(p => (
        <div
          key={p.id}
          style={{
            position: 'absolute',
            top: p.top,
            left: p.left,
            lineHeight: 0,
          }}
        >
          {/* Cursor caret line */}
          <div style={{
            width: 2,
            height: '1.2em',
            background: p.color,
            borderRadius: 1,
            animation: 'collab-blink 1.2s step-end infinite',
          }} />
          {/* Name label above the caret */}
          <div style={{
            position: 'absolute',
            bottom: '1.2em',
            left: 0,
            background: p.color,
            color: '#fff',
            fontSize: 11,
            fontFamily: 'system-ui, sans-serif',
            fontWeight: 600,
            padding: '2px 5px',
            borderRadius: '3px 3px 3px 0',
            whiteSpace: 'nowrap',
            lineHeight: 1.4,
            userSelect: 'none',
          }}>
            {p.name}
          </div>
        </div>
      ))}
    </div>
  )
}

// ── Remote edit highlight overlay ──────────────────────────────────────────────
//
// Flashes a colored, fading rectangle over characters just inserted by another
// user, so concurrent typing is visually attributed instead of just appearing.

interface HighlightRect { id: number; top: number; left: number; width: number; color: string }

function RemoteEditOverlay({ taRef, highlights, content }: {
  taRef: React.RefObject<HTMLTextAreaElement>
  highlights: RemoteEdit[]
  content: string
}) {
  const [rects, setRects] = useState<HighlightRect[]>([])

  useLayoutEffect(() => {
    const ta = taRef.current
    if (!ta || highlights.length === 0) { setRects([]); return }

    const computed = highlights.flatMap(h => {
      try {
        const start = getCaretCoords(ta, h.pos)
        const end = getCaretCoords(ta, h.pos + 1)
        const sameLine = end.top === start.top && end.left > start.left
        return [{
          id: h.id,
          top: start.top,
          left: start.left,
          width: sameLine ? end.left - start.left : 8,
          color: avatarColor(h.userId),
        }]
      } catch {
        return []
      }
    })
    setRects(computed)
  }, [highlights, content, taRef])

  if (rects.length === 0) return null

  return (
    <div style={{
      position: 'absolute',
      inset: 0,
      pointerEvents: 'none',
      overflow: 'hidden',
    }}>
      {rects.map(r => (
        <div
          key={r.id}
          style={{
            position: 'absolute',
            top: r.top,
            left: r.left,
            width: r.width,
            height: '1.2em',
            background: r.color,
            borderRadius: 2,
            animation: `collab-highlight-fade ${HIGHLIGHT_DURATION_MS}ms ease-out forwards`,
          }}
        />
      ))}
    </div>
  )
}

// ── Mirror technique: calculate pixel position of a character offset ──────────

function getCaretCoords(ta: HTMLTextAreaElement, pos: number): { top: number; left: number } {
  const clamped = Math.max(0, Math.min(pos, ta.value.length))
  const style = window.getComputedStyle(ta)

  const div = document.createElement('div')

  for (const prop of [
    'boxSizing', 'width',
    'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
    'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
    'fontFamily', 'fontSize', 'fontStyle', 'fontWeight', 'fontVariant',
    'lineHeight', 'letterSpacing', 'textTransform', 'tabSize',
  ] as const) {
    (div.style as unknown as Record<string, string>)[prop] = (style as unknown as Record<string, string>)[prop]
  }

  div.style.position = 'absolute'
  div.style.top = '0'
  div.style.left = '-9999px'
  div.style.visibility = 'hidden'
  div.style.whiteSpace = 'pre-wrap'
  div.style.wordBreak = 'break-word'
  div.style.overflowWrap = 'break-word'
  div.style.overflow = 'hidden'

  div.appendChild(document.createTextNode(ta.value.substring(0, clamped)))

  const marker = document.createElement('span')
  marker.textContent = '​' // zero-width space marks the cursor position
  div.appendChild(marker)

  // Include the rest so word-wrap matches the textarea exactly
  div.appendChild(document.createTextNode(ta.value.substring(clamped)))

  document.body.appendChild(div)

  const coords = {
    top: marker.offsetTop - ta.scrollTop,
    left: marker.offsetLeft - ta.scrollLeft,
  }

  document.body.removeChild(div)
  return coords
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function applyOp(content: string, op: Op): string {
  const chars = [...(content ?? '')]
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

function diffToOps(oldStr: string, newStr: string): Op[] {
  const ops: Op[] = []
  const old = oldStr ?? ''
  const next = newStr ?? ''

  let i = 0
  const minLen = Math.min(old.length, next.length)
  while (i < minLen && old[i] === next[i]) i++

  let oi = old.length - 1
  let ni = next.length - 1
  while (oi >= i && ni >= i && old[oi] === next[ni]) { oi--; ni-- }

  for (let k = oi; k >= i; k--) {
    ops.push({ type: 'delete', pos: k })
  }
  for (let k = i; k <= ni; k++) {
    ops.push({ type: 'insert', pos: k, char: next[k] })
  }

  return ops
}
