export function registerQuickEntryServiceWorker() {
  if (!import.meta.env.PROD) return;
  if (typeof window === 'undefined' || !('serviceWorker' in navigator)) return;
  void navigator.serviceWorker.register('/quick-entry-sw.js', { scope: '/quick-entry' }).catch(() => {
    // installability is best-effort; the main app should keep working without SW registration.
  });
}
