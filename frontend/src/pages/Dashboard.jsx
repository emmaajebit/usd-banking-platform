import { useEffect, useState } from 'react'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'

export default function Dashboard() {
  const { user } = useAuth()
  const [accounts, setAccounts] = useState([])
  const [notices, setNotices] = useState([])
  const [error, setError] = useState('')

  useEffect(() => {
    api.get('/accounts')
      .then(async (d) => {
        const items = d.items || []
        const withBal = await Promise.all(items.map(async (a) => {
          try {
            const b = await api.get(`/accounts/${a.id}/balance`)
            return { ...a, balance: `${b.balance} ${b.currency}` }
          } catch {
            return { ...a, balance: 'unavailable' }
          }
        }))
        setAccounts(withBal)
      })
      .catch((e) => setError(e.message))
    api.get('/notifications').then((d) => setNotices(d.notifications || d.items || [])).catch(() => {})
  }, [])

  const total = accounts.reduce((sum, a) => sum + Number(String(a.balance || '').split(' ')[0] || 0), 0)

  return (
    <div>
      <h1>Welcome back</h1>
      <p className="muted">{user?.email}</p>
      {error && <div className="error">{error}</div>}
      <div className="stat-row">
        <div className="card stat">
          <div className="stat-label">USD balance</div>
          <div className="stat-value">{Number.isFinite(total) ? total.toFixed(2) : '—'}</div>
        </div>
        <div className="card stat">
          <div className="stat-label">Accounts</div>
          <div className="stat-value">{accounts.length}</div>
        </div>
      </div>
      <h2>Your accounts</h2>
      {accounts.length === 0 ? (
        <p className="muted">No accounts yet. Open one from the Accounts page.</p>
      ) : (
        <div className="list">
          {accounts.map((a) => (
            <div key={a.id} className="card row">
              <div>
                <div className="mono">{a.account_number}</div>
                <div className="muted">{a.type} · {a.currency}</div>
              </div>
              <span className="badge ok">{a.balance}</span>
            </div>
          ))}
        </div>
      )}
      <h2>Notices</h2>
      {notices.length === 0 ? <p className="muted">No notices yet. Deposits and transfers show up here.</p> : (
        <div className="list">
          {notices.slice(0, 5).map((n) => (
            <div key={n.id} className="card"><div>{n.message}</div></div>
          ))}
        </div>
      )}
    </div>
  )
}
