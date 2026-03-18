self.addEventListener('install', () => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', (event) => {
  const request = event.request;
  if (request.mode !== 'navigate') return;
  const url = new URL(request.url);
  if (!url.pathname.startsWith('/quick-entry')) return;
  event.respondWith(fetch(request).catch(() => caches.match('/quick-entry') || caches.match('/index.html')));
});
