import { useEffect, useState } from 'react'
import { api } from '../api.js'

export default function Cards() {
  const [cards, setCards] = useState([])
  const [accounts, setAccounts] = useState([])
  const [accountId, setAccountId] = useState('')
  const [type, setType] = useState('DEBIT')
  const [error, setError] = useState('')

  async function load() {
    try {
      setCards((await api.get('/cards')).items || [])
    } catch (e) { setError(e.message) }
  }

  useEffect(() => {
    load()
    api.get('/accounts').then((d) => setAccounts(d.items || [])).catch(() => {})
  }, [])

  async function issue(e) {
    e.preventDefault()
    setError('')
    try {
      await api.post('/cards', { account_id: accountId, type })
      await load()
    } catch (e) { setError(e.message) }
  }

  const [spendAmount, setSpendAmount] = useState('')

  async function spend(card) {
    setError('')
    try {
      await api.post(`/cards/${card.id}/spend`, { amount: spendAmount }, { 'Idempotency-Key': crypto.randomUUID() })
      setSpendAmount('')
    } catch (e) { setError(e.message) }
  }
    const action = card.status === 'ACTIVE' ? 'block' : 'unblock'
    try {
      await api.post(`/cards/${card.id}/${action}`)
      await load()
    } catch (e) { setError(e.message) }
  }

  return (
    <div>
      <h1>Cards</h1>
      {error && <div className="error">{error}</div>}
      <form className="card inline-form" onSubmit={issue}>
        <label>Account
          <select value={accountId} onChange={(e) => setAccountId(e.target.value)} required>
            <option value="">Select…</option>
            {accounts.map((a) => <option key={a.id} value={a.id}>{a.account_number}</option>)}
          </select>
        </label>
        <label>Type
          <select value={type} onChange={(e) => setType(e.target.value)}>
            <option>DEBIT</option><option>CREDIT</option>
          </select>
        </label>
        <button type="submit">Issue card</button>
      </form>
      <form className="card inline-form" onSubmit={(e) => e.preventDefault()}>
        <label>Debit spend amount
          <input value={spendAmount} onChange={(e) => setSpendAmount(e.target.value)} placeholder="10.00" />
        </label>
      </form>

      <div className="list">
        {cards.map((c) => (
          <div key={c.id} className="card row">
            <div>
              <div className="mono">{c.masked_number}</div>
              <div className="muted">{c.network} · {c.type} · exp {c.expiry_month}/{c.expiry_year}</div>
            </div>
            <div className="row-actions">
              <span className={`badge ${c.status === 'ACTIVE' ? 'ok' : 'warn'}`}>{c.status}</span>
              <button onClick={() => toggle(c)}>{c.status === 'ACTIVE' ? 'Block' : 'Unblock'}</button>
              {c.type === 'DEBIT' && c.status === 'ACTIVE' && <button onClick={() => spend(c)}>Spend</button>}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
