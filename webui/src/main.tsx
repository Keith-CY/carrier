import React from 'react';
import ReactDOM from 'react-dom/client';
import { RouterProvider } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { PWAProvider } from './app/pwa';
import { registerQuickEntryServiceWorker } from './app/registerServiceWorker';
import { createAppRouter } from './app/routes';
import { SessionProvider } from './app/session';
import './index.css';

const queryClient = new QueryClient();
const router = createAppRouter();
registerQuickEntryServiceWorker();

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <PWAProvider>
        <SessionProvider>
          <RouterProvider router={router} />
        </SessionProvider>
      </PWAProvider>
    </QueryClientProvider>
  </React.StrictMode>,
);
