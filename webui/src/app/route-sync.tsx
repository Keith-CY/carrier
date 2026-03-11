import { useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

function normalizeLegacyHash(hash: string): string | null {
  const text = String(hash || '').trim();
  if (!text.startsWith('#/')) return null;
  return text.slice(1);
}

export function RouteSync() {
  const location = useLocation();
  const navigate = useNavigate();

  useEffect(() => {
    const hashTarget = normalizeLegacyHash(window.location.hash);
    if (hashTarget && hashTarget !== location.pathname + location.search) {
      navigate(hashTarget, { replace: true });
    }
  }, [location.pathname, location.search, navigate]);

  return null;
}
