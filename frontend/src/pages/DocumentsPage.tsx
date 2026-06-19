import Sidebar from '../components/Sidebar'

export default function DocumentsPage() {
  return (
    <div className="app-layout">
      <Sidebar />
      <main className="home-area">
        <h2>Selecione ou crie uma página</h2>
        <p>Clique em uma página na barra lateral ou crie uma nova.</p>
      </main>
    </div>
  )
}
