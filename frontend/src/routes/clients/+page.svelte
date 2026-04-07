<script lang="ts">
  import { hubStore } from '$lib/stores/hub';
  import { onMount } from 'svelte';
  
  let clients = $state<any[]>([]);
  let loading = $state(true);
  
  onMount(async () => {
    try {
      const resp = await fetch('http://127.0.0.1:4117/api/v1/clients');
      if (resp.ok) {
        const data = await resp.json();
        clients = data.clients || [];
      }
    } catch (e) {
      console.error('Failed to load clients:', e);
    }
    loading = false;
  });
  
  const kindColors: Record<string, string> = {
    gui: 'bg-indigo-500/10 text-indigo-400 border-indigo-500/20',
    cli: 'bg-purple-500/10 text-purple-400 border-purple-500/20'
  };
</script>

<div class="space-y-6 animate-fade-in">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div>
      <p class="text-zinc-400">Manage IDE and app configurations</p>
    </div>
    <a 
      href="/sync"
      class="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-gradient-to-r from-indigo-500 to-purple-500 text-white font-medium hover:from-indigo-600 hover:to-purple-600 transition-all shadow-lg shadow-indigo-500/25"
    >
      <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
      </svg>
      Sync All
    </a>
  </div>
  
  {#if loading}
    <!-- Loading skeletons -->
    <div class="glass rounded-2xl overflow-hidden">
      {#each Array(3) as _}
        <div class="px-5 py-4 border-b border-white/5 flex items-center gap-4">
          <div class="w-10 h-10 rounded-xl bg-white/5 animate-pulse"></div>
          <div class="flex-1 space-y-2">
            <div class="h-4 bg-white/5 rounded w-1/4 animate-pulse"></div>
            <div class="h-3 bg-white/5 rounded w-1/2 animate-pulse"></div>
          </div>
        </div>
      {/each}
    </div>
  {:else if clients.length === 0}
    <!-- Empty state -->
    <div class="glass rounded-3xl p-12 text-center border border-dashed border-white/10">
      <div class="w-20 h-20 rounded-3xl bg-indigo-500/10 flex items-center justify-center mx-auto mb-6">
        <svg class="w-10 h-10 text-indigo-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" />
        </svg>
      </div>
      <h3 class="text-xl font-semibold text-white mb-2">No clients configured</h3>
      <p class="text-zinc-400">Clients are defined in your config.yaml file.</p>
    </div>
  {:else}
    <!-- Clients list -->
    <div class="glass rounded-2xl border border-white/5 overflow-hidden">
      <table class="w-full">
        <thead>
          <tr class="border-b border-white/5 bg-black/20">
            <th class="text-left px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Client</th>
            <th class="text-left px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Kind</th>
            <th class="text-left px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Config Path</th>
            <th class="text-left px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Servers</th>
            <th class="text-right px-5 py-4 text-xs font-medium text-zinc-500 uppercase tracking-wider">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-white/5">
          {#each clients as client, i}
            <tr class="hover:bg-white/[0.02] transition-colors" style="animation-delay: {i * 50}ms">
              <td class="px-5 py-4">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-indigo-500/20 to-purple-500/20 flex items-center justify-center">
                    <svg class="w-5 h-5 text-indigo-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                    </svg>
                  </div>
                  <span class="font-medium text-white">{client.displayName || client.id}</span>
                </div>
              </td>
              <td class="px-5 py-4">
                <span class="text-xs px-2.5 py-1 rounded-full border capitalize {kindColors[client.kind] || kindColors.gui}">
                  {client.kind || 'gui'}
                </span>
              </td>
              <td class="px-5 py-4">
                <span class="text-sm text-zinc-400 font-mono">{client.configPath || '-'}</span>
              </td>
              <td class="px-5 py-4">
                <div class="flex items-center gap-2">
                  <span class="text-lg font-semibold text-white">{client.serverCount || 0}</span>
                  <span class="text-xs text-zinc-500">servers</span>
                </div>
              </td>
              <td class="px-5 py-4 text-right">
                <a href="/sync" class="text-sm text-indigo-400 hover:text-indigo-300 transition-colors">Sync</a>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
