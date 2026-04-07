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
