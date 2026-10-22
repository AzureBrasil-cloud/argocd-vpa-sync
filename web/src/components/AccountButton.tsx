import { useRef, useState } from 'react'
import { LogOut, UserRound } from 'lucide-react'

interface AccountButtonProps {
  username?: string
  onSignOut: () => Promise<void>
}

/**
 * The default theme's session control: the user icon with "Signed in as"
 * and the username at the foot of the sidebar (the text shows when the
 * sidebar expands), opening a small dialog to confirm signing out. A native
 * <dialog> gives the focus trap, Escape to close and the backdrop; a click
 * on the backdrop closes it too.
 */
export function AccountButton({ username, onSignOut }: AccountButtonProps) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [busy, setBusy] = useState(false)
  const name = username || 'admin'

  async function signOut() {
    setBusy(true)
    await onSignOut()
  }

  return (
    <>
      <button className="account-button" aria-haspopup="dialog" onClick={() => dialog.current?.showModal()}>
        <span className="account-badge" aria-hidden="true">
          <UserRound className="account-icon" size={18} strokeWidth={1.75} />
        </span>
        <span className="account-text">
          <span className="account-label">Signed in as</span>
          <span className="account-name">{name}</span>
        </span>
      </button>

      <dialog
        ref={dialog}
        className="dialog"
        aria-labelledby="account-dialog-title"
        onClick={(event) => {
          if (event.target === dialog.current) dialog.current.close()
        }}
      >
        <div className="dialog-body">
          <div className="account-identity">
            <span className="account-avatar" aria-hidden="true">
              <UserRound size={20} strokeWidth={1.75} />
            </span>
            <div>
              <p className="account-label">Signed in as</p>
              <h2 id="account-dialog-title" className="account-name">
                {name}
              </h2>
            </div>
          </div>
          <p className="dialog-text">Signing out ends your session in this browser.</p>
          <div className="dialog-actions">
            <button className="btn" autoFocus disabled={busy} onClick={() => dialog.current?.close()}>
              Cancel
            </button>
            <button className="btn btn-primary" disabled={busy} onClick={signOut}>
              <LogOut size={16} strokeWidth={1.75} aria-hidden="true" />
              {busy ? 'Signing out…' : 'Sign out'}
            </button>
          </div>
        </div>
      </dialog>
    </>
  )
}
