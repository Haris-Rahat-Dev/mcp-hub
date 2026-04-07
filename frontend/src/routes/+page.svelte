<script lang="ts">
  import { onMount } from 'svelte';
  import { hubStore, serverCount } from '$lib/stores/hub';
  import { toastStore } from '$lib/stores/toast';
  import StatusBadge from '$lib/components/StatusBadge.svelte';
  
  let loading = $state(true);
  let refreshing = $state(false);
  
  onMount(() => {
    loadStatus();
    const interval = setInterval(loadStatus, 5000);
    return () => clearInterval(interval);
  });
  
  async function loadStatus() {
    if (refreshing) return;
    try {
      const resp = await fetch('http://127.0.0.1:4117/api/v1/status');
      if (resp.ok) {
        const status = await resp.json();
        hubStore.setStatus(status);
      }
    } catch (e) {
      // Hub not reachable
    }
    loading = false;
  }
  
  async function handleRefresh() {
    refreshing = true;
    await loadStatus();
    refreshing = false;
  }
</script>

<div class="space-y-8 animate-fade-in">
  <!-- Stats cards -->
  <div class="grid grid-cols-3 gap-4">
    <!-- Running -->
    <div class="glass rounded-2xl p-6 border border-green-500/10 hover-lift transition-all duration-300">
      <div class="flex items-center justify-between mb-4">
        <div class="w-12 h-12 rounded-2xl bg-green-500/10 flex items-center justify-center">
          <svg class="w-6 h-6 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <button 
          class="p-2 rounded-xl hover:bg-white/5 text-zinc-500 hover:text-white transition-all"
          onclick={handleRefresh}
          disabled={refreshing}
        >
          <svg class="w-4 h-4 {refreshing ? 'animate-spin' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
        </button>
      </div>
      <p class="text-4xl font-bold text-white mb-1">{$serverCount.running}</p>
      <p class="text-sm text-zinc-400">Servers Running</p>
      <div class="mt-4 h-1 rounded-full bg-green-500/20 overflow-hidden">
        <div 
          class="h-full bg-gradient-to-r from-green-500 to-emerald-500 transition-all duration-500"
          style="width: {$hubStore.servers.length > 0 ? ($serverCount.running / $hubStore.servers.length * 100) : 0}%"
        ></div>
      </div>
    </div>
    
    <!-- Idle -->
    <div class="glass rounded-2xl p-6 border border-amber-500/10 hover-lift transition-all duration-300">
      <div class="flex items-center justify-between mb-4">
        <div class="w-12 h-12 rounded-2xl bg-amber-500/10 flex items-center justify-center">
          <svg class="w-6 h-6 text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
      </div>
      <p class="text-4xl font-bold text-white mb-1">{$serverCount.idle}</p>
      <p class="text-sm text-zinc-400">Servers Idle</p>
      <div class="mt-4 h-1 rounded-full bg-amber-500/20 overflow-hidden">
        <div 
          class="h-full bg-gradient-to-r from-amber-500 to-orange-500 transition-all duration-500"
          style="width: {$hubStore.servers.length > 0 ? ($serverCount.idle / $hubStore.servers.length * 100) : 0}%"
        ></div>
      </div>
    </div>
    
    <!-- Errors -->
    <div class="glass rounded-2xl p-6 border border-red-500/10 hover-lift transition-all duration-300">
      <div class="flex items-center justify-between mb-4">
        <div class="w-12 h-12 rounded-2xl bg-red-500/10 flex items-center justify-center">
          <svg class="w-6 h-6 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
        </div>
      </div>
      <p class="text-4xl font-bold text-white mb-1">{$serverCount.error}</p>
      <p class="text-sm text-zinc-400">Servers with Errors</p>
      <div class="mt-4 h-1 rounded-full bg-red-500/20 overflow-hidden">
        <div 
          class="h-full bg-gradient-to-r from-red-500 to-rose-500 transition-all duration-500"
          style="width: {$hubStore.servers.length > 0 ? Math.max($serverCount.error / $hubStore.servers.length * 100, 2) : 2}%"
        ></div>
      </div>
    </div>
  </div>
  
  {#if loading}
    <!-- Loading skeletons -->
    <div class="grid grid-cols-3 gap-4">
      {#each Array(6) as _, i}
        <div class="glass rounded-2xl p-5 animate-pulse">
          <div class="h-4 bg-white/5 rounded w-1/3 mb-3"></div>
          <div class="h-8 bg-white/5 rounded w-1/2 mb-2"></div>
          <div class="h-3 bg-white/5 rounded w-2/3"></div>
        </div>
      {/each}
    </div>
  {:else if $hubStore.servers.length === 0}
    <!-- Empty state -->
    <div class="glass rounded-3xl p-12 text-center border border-dashed border-white/10">
      <div class="w-20 h-20 rounded-3xl bg-indigo-500/10 flex items-center justify-center mx-auto mb-6">
        <svg class="w-10 h-10 text-indigo-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2m-14 0a2 2 0 00-2 2v4a2 2 0 002 2m14 0V6a2 2 0 00-2-2h-4a2 2 0 00-2 2v4" />
        </svg>
      </div>
      <h3 class="text-xl font-semibold text-white mb-2">No servers configured</h3>
      <p class="text-zinc-400 mb-6 max-w-md mx-auto">Get started by adding your first MCP server to the hub configuration.</p>
      <a 
        href="/servers"
        class="inline-flex items-center gap-2 px-6 py-3 rounded-xl bg-gradient-to-r from-indigo-500 to-purple-500 text-white font-medium hover:from-indigo-600 hover:to-purple-600 transition-all shadow-lg shadow-indigo-500/25"
      >
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        Add Server
      </a>
    </div>
  {:else}
    <!-- Servers list -->
    <div>
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold text-white">All Servers</h3>
        <span class="text-sm text-zinc-500">{$hubStore.servers.length} total</span>
      </div>
      
      <div class="glass rounded-2xl border border-white/5 overflow-hidden">
        <table class="w-full">
          <thead>
            <tr class="border-b border-white/5">
              <th class="text-left px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Server</th>
              <th class="text-left px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Status</th>
              <th class="text-left px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Mode</th>
              <th class="text-left px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Clients</th>
              <th class="text-left px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Latency</th>
              <th class="text-right px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-white/5">
            {#each $hubStore.servers as server, i}
              <tr class="hover:bg-white/[0.02] transition-colors" style="animation-delay: {i * 50}ms">
                <td class="px-5 py-4">
                  <div class="flex items-center gap-3">
                    <span class="font-medium text-white">{server.name}</span>
                  </div>
                </td>
                <td class="px-5 py-4">
                  <StatusBadge status={server.status} />
                </td>
                <td class="px-5 py-4">
                  <span class="text-xs px-2.5 py-1 rounded-full bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 capitalize">
                    {server.mode}
                  </span>
                </td>
                <td class="px-5 py-4 text-zinc-400">
                  {server.clientCount || 0}
                </td>
                <td class="px-5 py-4">
                  {#if server.lastPingMs}
                    <span class="{server.lastPingMs < 100 ? 'text-green-400' : server.lastPingMs < 500 ? 'text-amber-400' : 'text-red-400'}">
                      {server.lastPingMs}ms
                    </span>
                  {:else}
                    <span class="text-zinc-600">-</span>
                  {/if}
                </td>
                <td class="px-5 py-4 text-right">
                  <a href="/servers" class="text-sm text-indigo-400 hover:text-indigo-300 transition-colors">View</a>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
    
    <!-- Error servers alert -->
    {#if $serverCount.error > 0}
      <div class="glass rounded-2xl p-6 border border-red-500/20 bg-red-500/5">
        <div class="flex items-start gap-4">
          <div class="w-10 h-10 rounded-xl bg-red-500/10 flex items-center justify-center shrink-0">
            <svg class="w-5 h-5 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <div class="flex-1">
            <h4 class="font-medium text-white mb-1">Servers requiring attention</h4>
            <div class="space-y-2">
              {#each $hubStore.servers.filter(s => s.status === 'error') as server}
                <div class="flex items-center justify-between">
                  <span class="text-sm text-zinc-400">{server.name}</span>
                  <span class="text-xs text-red-400">{server.lastError}</span>
                </div>
              {/each}
            </div>
          </div>
        </div>
      </div>
    {/if}
  {/if}
</div>
