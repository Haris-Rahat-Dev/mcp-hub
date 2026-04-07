// @ts-check
import { ListServers, Status, SaveConfig, ReloadConfig } from './wailsjs/go/gui/App';

export interface Server {
  name: string;
  mode: 'shared' | 'isolated' | 'passthrough';
  status: 'running' | 'idle' | 'error' | 'stopped' | 'starting' | 'restarting';
  command?: string;
  args?: string[];
  url?: string;
  lazy: boolean;
  pid?: number;
  ramMB?: number;
  clientCount: number;
  uptime?: string;
  lastError?: string;
  restarts?: number;
  lastPingMs?: number;
  lastPingAt?: string;
}

export interface Client {
  id: string;
  displayName: string;
  kind: 'gui' | 'cli';
  configPath: string;
  syncedAt?: string;
  serverCount: number;
  lastError?: string;
}

export interface HubStatus {
  version: string;
  address: string;
  servers: Server[];
  clients: Client[];
  uptime?: string;
  totalRAM?: string;
  requiresAuth?: boolean;
}

export interface SyncResult {
  clientID: string;
  ok: boolean;
  changes: number;
  errors?: string;
  dryRun: boolean;
}

export interface LogEntry {
  time: string;
  level: 'debug' | 'info' | 'warn' | 'error';
  server: string;
  message: string;
}

export interface Toast {
  id: string;
  type: 'success' | 'error' | 'info' | 'warn';
  message: string;
}

export const api = {
  async getStatus(): Promise<HubStatus | null> {
    try {
      return await Status() as HubStatus;
    } catch (e) {
      console.error('Failed to get status:', e);
      return null;
    }
  },

  async listServers(): Promise<Server[]> {
    try {
      const servers = await ListServers();
      return (servers || []).map((s: any) => ({
        name: s.name || '',
        mode: s.mode || 'shared',
        status: s.status || 'idle',
        command: s.command,
        args: s.args,
        url: s.url,
        lazy: s.lazy ?? true,
        pid: s.pid,
        ramMB: s.ramMB,
        clientCount: s.clientCount ?? 0,
        uptime: s.uptime,
        lastError: s.lastError,
        restarts: s.restarts,
        lastPingMs: s.lastPingMs,
        lastPingAt: s.lastPingAt
      }));
    } catch (e) {
      console.error('Failed to list servers:', e);
      return [];
    }
  },

  async saveConfig(yaml: string): Promise<void> {
    await SaveConfig(yaml);
  },

  async reloadConfig(): Promise<void> {
    await ReloadConfig();
  }
};
