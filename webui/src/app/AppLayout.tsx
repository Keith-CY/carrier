import { NavLink, Outlet } from 'react-router-dom';
import { primaryNav, secondaryNav } from './navigation';
import { RouteSync } from './route-sync';
import { useFeatures } from './useFeatures';
import { useSession } from './session';

function isNavItemVisible(
  route: string,
  featureFlags: { remoteControlPlaneEnabled: boolean; remoteChatEnabled: boolean },
) {
  switch (route) {
    case 'work':
    case 'executions':
    case 'workers':
    case 'hosts':
    case 'providers':
    case 'policies':
    case 'remote-observability':
      return featureFlags.remoteControlPlaneEnabled;
    case 'remote-chat':
      return featureFlags.remoteControlPlaneEnabled && featureFlags.remoteChatEnabled;
    default:
      return true;
  }
}

function renderNav(
  items: { to: string; route: string; label: string }[],
  featureFlags: { remoteControlPlaneEnabled: boolean; remoteChatEnabled: boolean },
) {
  return items.map((item) => (
    isNavItemVisible(item.route, featureFlags) ? (
    <NavLink key={item.route} to={item.to} className="nav-link" data-route={item.route}>
      <span className="nav-text">{item.label}</span>
    </NavLink>
    ) : null
  ));
}

export function AppLayout() {
  const { authenticated, clearLoginError, dismissToast, health, login, loginError, logout, toasts } = useSession();
  const { featureFlags } = useFeatures(authenticated);

  return (
    <>
      <div id="login-overlay" className={`overlay${authenticated ? ' hidden' : ''}`}>
        <div className="card login-card">
          <h2>Carrier</h2>
          <p className="text-dim">Enter your Gateway token to continue.</p>
          <input
            type="password"
            id="login-token"
            placeholder="Gateway Token"
            autoComplete="off"
            onChange={() => clearLoginError()}
            onKeyDown={(event) => {
              if (event.key !== 'Enter') return;
              event.preventDefault();
              const target = event.currentTarget;
              void login(target.value);
            }}
          />
          <div id="login-msg" className={loginError ? 'msg-error' : ''}>{loginError}</div>
          <button
            id="login-btn"
            type="button"
            onClick={() => {
              const field = document.getElementById('login-token') as HTMLInputElement | null;
              void login(field?.value || '');
            }}
          >
            Connect
          </button>
        </div>
      </div>

      <div id="app">
        <header id="header">
          <div className="header-left">
            <h1>Carrier</h1>
            <nav id="nav" className={authenticated ? '' : 'hidden'}>
              {renderNav(primaryNav, featureFlags)}
              {renderNav(secondaryNav, featureFlags)}
            </nav>
          </div>
          <div className="header-right">
            <span id="health-badge" className={health.className}>{health.text}</span>
            <button id="logout-btn" className={`btn-sm btn-secondary${authenticated ? '' : ' hidden'}`} type="button" onClick={() => logout()}>
              Logout
            </button>
          </div>
        </header>

        <RouteSync />
        <main id="main">
          <Outlet />
        </main>
      </div>
      {toasts.length ? (
        <div id="delegate-toast-root" className="delegate-toast-root">
          {toasts.map((toast) => (
            <div key={toast.id} className={`delegate-toast-item ${String(toast.status || '').trim().toLowerCase() === 'completed' ? 'success' : 'error'}`}>
              <span>{toast.text}</span>
              <button type="button" className="btn-sm btn-secondary" onClick={() => dismissToast(toast.id)}>Dismiss</button>
            </div>
          ))}
        </div>
      ) : null}
    </>
  );
}
