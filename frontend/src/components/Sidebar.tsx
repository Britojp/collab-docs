import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, DocumentSummary } from '../services/api'

interface Props {
  onDocumentsChange?: (docs: DocumentSummary[]) => void
}

export default function Sidebar({ onDocumentsChange }: Props) {
  const navigate = useNavigate()
  const { id: activeId } = useParams<{ id?: string }>()
  const [docs, setDocs] = useState<DocumentSummary[]>([])
  const [creating, setCreating] = useState(false)
  const [newTitle, setNewTitle] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const userName = localStorage.getItem('userName') ?? 'Usuário'
  const myUserId = localStorage.getItem('userId') ?? ''

  useEffect(() => {
    api.listDocuments().then(d => {
      setDocs(d)
      onDocumentsChange?.(d)
    })
  }, [])

  useEffect(() => {
    if (creating) inputRef.current?.focus()
  }, [creating])

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    const title = newTitle.trim() || 'Sem título'
    setCreating(false)
    setNewTitle('')
    try {
      const doc = await api.createDocument(title)
      setDocs(d => [doc, ...d])
      navigate(`/documents/${doc.id}`)
    } catch { /* ignore */ }
  }

  async function handleDelete(e: React.MouseEvent, id: string) {
    e.stopPropagation()
    await api.deleteDocument(id)
    setDocs(d => d.filter(doc => doc.id !== id))
    if (activeId === id) navigate('/documents')
  }

  function handleLogout() {
    localStorage.clear()
    navigate('/login')
  }

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <div className="sidebar-workspace">
          <div className="sidebar-workspace-icon">C</div>
          <span className="sidebar-workspace-name">CollabDocs</span>
        </div>
      </div>

      <div className="sidebar-divider" />

      <div className="sidebar-docs">
        <p className="sidebar-section-label">Páginas</p>

        {docs.map(doc => (
          <button
            key={doc.id}
            className={`sidebar-doc-item ${doc.id === activeId ? 'active' : ''}`}
            onClick={() => navigate(`/documents/${doc.id}`)}
          >
            <span className="sidebar-doc-name">
              {doc.title}
            </span>
            {doc.ownerId === myUserId && (
              <span
                className="sidebar-doc-delete"
                onClick={e => handleDelete(e, doc.id)}
                title="Excluir"
              >
                ✕
              </span>
            )}
          </button>
        ))}

        {creating ? (
          <form className="sidebar-new-input" onSubmit={handleCreate}>
            <input
              ref={inputRef}
              value={newTitle}
              onChange={e => setNewTitle(e.target.value)}
              onBlur={handleCreate}
              placeholder="Nome da página..."
            />
          </form>
        ) : (
          <button
            className="sidebar-new-btn"
            onClick={() => setCreating(true)}
          >
            <span style={{ fontSize: 16 }}>+</span>
            Nova página
          </button>
        )}
      </div>

      <div className="sidebar-footer">
        <div className="sidebar-user">
          <span className="sidebar-user-name">{userName}</span>
          <button className="sidebar-logout" onClick={handleLogout}>Sair</button>
        </div>
      </div>
    </aside>
  )
}
