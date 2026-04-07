<script lang="ts">
  import type { Server } from '$lib/api';
  import StatusBadge from './StatusBadge.svelte';
  
  interface Props {
    server: Server;
    onremove?: (name: string) => void;
  }
  
  let { server, onremove }: Props = $props();
  
  function getTarget(srv: Server): string {
    if (srv.url) return srv.url;
    if (srv.command) {
      const cmd = srv.command.split('/').pop() || srv.command;
      return srv.args?.length ? `${cmd} ${srv.args.join(' ')}` : cmd;
    }
    return '-';
  }
  
  const modeColors: Record<string, string> = {
    shared: 'bg-indigo-500/10 text-indigo-400 border-indigo-500/20',
    isolated: 'bg-purple-500/10 text-purple-400 border-purple-500/20',
    passthrough: 'bg-cyan-500/10 text-cyan-400 border-cyan-500/20'
  };
</script>

<div class="group glass rounded-2xl p-5 hover-lift border border-white/5 transition-all duration-300 hover:border-white/10">
  <div class="flex items-start justify-between mb-4">
    <div class="flex-1 min-w-0">
      <div class="flex items-center gap-3 mb-1">
        <h3 class="font-semibold text-white truncate">{server.name}</h3>
        <StatusBadge status={server.status} size="sm" />
      </div>
      <p class="text-xs text-zinc-500 font-mono truncate" title={getTarget(server)}>
        {getTarget(server).slice(0, 40)}{getTarget(server).length > 40 ? '...' : ''}
      </p>
    </div>
    
    {#if onremove}
      <button
        class="opacity-0 group-hover:opacity-100 p-2 rounded-lg hover:bg-red-500/10 text-zinc-500 hover:text-red-400 transition-all duration-200"
        onclick={() => onremove(server.name)}
        title="Remove server"
      >
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
        </svg>
      </button>
    {/if}
  </div>
  
  <!-- Stats grid -->
  <div class="grid grid-cols-3 gap-3 mb-4">
    <div class="bg-white/5 rounded-xl p-3 text-center">
      <p class="text-lg font-semibold text-white">{server.clientCount || 0}</p>
      <p class="text-xs text-zinc-500">Clients</p>
    </div>
    {#if server.pid}
      <div class="bg-white/5 rounded-xl p-3 text-center">
        <p class="text-lg font-semibold text-white font-mono">{server.pid}</p>
        <p class="text-xs text-zinc-500">PID</p>
      </div>
    {/if}
    {#if server.lastPingMs}
      <div class="bg-white/5 rounded-xl p-3 text-center">
        <p class="text-lg font-semibold {server.lastPingMs < 100 ? 'text-green-400' : server.lastPingMs < 500 ? 'text-amber-400' : 'text-red-400'}">{server.lastPingMs}ms</p>
        <p class="text-xs text-zinc-500">Latency</p>
      </div>
    {:else}
      <div class="bg-white/5 rounded-xl p-3 text-center">
        <p class="text-lg font-semibold text-zinc-400">{server.lazy ? 'Lazy' : 'Eager'}</p>
        <p class="text-xs text-zinc-500">Start</p>
      </div>
    {/if}
  </div>
  
  <!-- Mode badge -->
  <div class="flex items-center justify-between">
    <span class="text-xs px-2.5 py-1 rounded-full border {modeColors[server.mode] || modeColors.shared}">
      {server.mode}
    </span>
    
    {#if server.lastError}
      <span class="text-xs text-red-400 truncate max-w-[120px]" title={server.lastError}>
        {server.lastError}
      </span>
    {/if}
  </div>
</div>
