import { writable, derived } from 'svelte/store';
import type { HubStatus, Server } from '$lib/api';

function createHubStore() {
  const { subscribe, set, update } = writable<HubStatus>({
    version: '',
    address: '',
    servers: [],
    clients: [],
    uptime: '',
    totalRAM: '',
    requiresAuth: false
  });

  return {
    subscribe,
    set,
    update,
    async refresh() {
      try {
        const resp = await fetch('/api/v1/status');
        if (resp.ok) {
          const data = await resp.json();
          set(data);
          return data;
        }
      } catch (e) {
        console.error('Failed to refresh status:', e);
      }
      return null;
    },
    setStatus(status: HubStatus) {
      set(status);
    },
    updateServer(name: string, fn: (s: Server) => Server) {
      update(s => {
        const idx = s.servers.findIndex(srv => srv.name === name);
        if (idx >= 0) {
          s.servers[idx] = fn(s.servers[idx]);
          s.servers = [...s.servers];
        }
        return s;
      });
    }
  };
}

export const hubStore = createHubStore();

export const serverCount = derived(hubStore, $hub => ({
  running: $hub.servers.filter(s => s.status === 'running').length,
  idle: $hub.servers.filter(s => s.status === 'idle').length,
  error: $hub.servers.filter(s => s.status === 'error').length
}));

export const serversByStatus = derived(hubStore, $hub => ({
  running: $hub.servers.filter(s => s.status === 'running'),
  idle: $hub.servers.filter(s => s.status === 'idle'),
  error: $hub.servers.filter(s => s.status === 'error'),
  stopped: $hub.servers.filter(s => ['stopped', 'starting', 'restarting'].includes(s.status))
}));
