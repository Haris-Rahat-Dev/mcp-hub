<script lang="ts">
  import { toastStore } from '$lib/stores/toast';
  
  let dryRun = $state(true);
  let syncing = $state(false);
  let results = $state<Array<{ client: string; status: string; changes: number; errors?: string }>>([]);
  
  async function syncAll() {
    syncing = true;
    results = [];
    
    try {
      const resp = await fetch('http://127.0.0.1:4117/api/v1/sync', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ client_id: '', dry_run: dryRun })
      });
      
      if (resp.ok) {
        const data = await resp.json();
        if (Array.isArray(data)) {
          results = data.map((r: any) => ({
            client: r.clientID || r.client_id || 'unknown',
            status: r.ok ? 'synced' : 'error',
            changes: r.changes || 0,
            errors: r.errors
          }));
        }
        
        if (dryRun) {
          toastStore.info('Dry run complete - no changes made');
        } else {
          toastStore.success('Sync complete');
        }
      } else {
        toastStore.error('Sync failed');
      }
    } catch (e) {
      toastStore.error('Sync failed');
    }
    syncing = false;
  }
</script>

<div class="space-y-6 animate-fade-in">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div>
      <p class="text-zinc-400">Sync hub configuration to IDEs</p>
    </div>
  </div>
  
  <!-- Controls -->
  <div class="glass rounded-2xl p-6 border border-white/5">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-4">
        <label class="flex items-center gap-3 cursor-pointer group">
          <div class="relative">
            <input type="checkbox" bind:checked={dryRun} class="sr-only peer" />
            <div class="w-11 h-6 bg-white/10 rounded-full peer peer-checked:bg-indigo-500/30 transition-colors"></div>
            <div class="absolute left-1 top-1 w-4 h-4 bg-zinc-500 rounded-full peer-checked:translate-x-5 peer-checked:bg-indigo-500 transition-all"></div>
          </div>
          <span class="text-sm text-zinc-300 group-hover:text-white transition-colors">Dry run</span>
        </label>
        
        {#if dryRun}
          <span class="text-xs px-2.5 py-1 rounded-full bg-amber-500/10 text-amber-400 border border-amber-500/20">
            No changes will be made
          </span>
        {:else}
          <span class="text-xs px-2.5 py-1 rounded-full bg-green-500/10 text-green-400 border border-green-500/20">
            Changes will be written
          </span>
        {/if}
      </div>
      
      <button
        class="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-gradient-to-r from-indigo-500 to-purple-500 text-white font-medium hover:from-indigo-600 hover:to-purple-600 transition-all shadow-lg shadow-indigo-500/25 disabled:opacity-50"
        onclick={syncAll}
        disabled={syncing}
      >
        {#if syncing}
          <svg class="w-5 h-5 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
          </svg>
          Syncing...
        {:else}
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          Sync All Clients
        {/if}
      </button>
    </div>
  </div>
  
  {#if results.length > 0}
    <!-- Results table -->
    <div class="glass rounded-2xl border border-white/5 overflow-hidden">
      <div class="px-5 py-4 border-b border-white/5">
        <h3 class="font-medium text-white">Sync Results</h3>
      </div>
      <table class="w-full">
        <thead>
          <tr class="border-b border-white/5 bg-black/20">
            <th class="text-left px-5 py-3 text-xs font-medium text-zinc-500 uppercase tracking-wider">Client</th>
            <th class="text-left px-5 py-3 text-xs font-medium text-zinc-500 uppercase tracking-wider">Status</th>
            <th class="text-left px-5 py-3 text-xs font-medium text-zinc-500 uppercase tracking-wider">Changes</th>
            <th class="text-left px-5 py-3 text-xs font-medium text-zinc-500 uppercase tracking-wider">Errors</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-white/5">
          {#each results as result}
            <tr class="hover:bg-white/[0.02] transition-colors">
              <td class="px-5 py-4">
                <div class="flex items-center gap-3">
                  {#if result.status === 'synced'}
                    <div class="w-8 h-8 rounded-lg bg-green-500/10 flex items-center justify-center">
                      <svg class="w-4 h-4 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                      </svg>
                    </div>
                  {:else}
                    <div class="w-8 h-8 rounded-lg bg-red-500/10 flex items-center justify-center">
                      <svg class="w-4 h-4 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                      </svg>
                    </div>
                  {/if}
                  <span class="font-medium text-white">{result.client}</span>
                </div>
              </td>
              <td class="px-5 py-4">
                <span class="text-xs px-2.5 py-1 rounded-full {result.status === 'synced' ? 'bg-green-500/10 text-green-400 border border-green-500/20' : 'bg-red-500/10 text-red-400 border border-red-500/20'}">
                  {result.status}
                </span>
              </td>
              <td class="px-5 py-4 text-zinc-400">
                {result.changes} server{result.changes !== 1 ? 's' : ''}
              </td>
              <td class="px-5 py-4">
                {#if result.errors}
                  <span class="text-xs text-red-400">{result.errors}</span>
                {:else}
                  <span class="text-zinc-600">-</span>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {:else}
    <!-- Empty state -->
    <div class="glass rounded-3xl p-12 text-center border border-dashed border-white/10">
      <div class="w-20 h-20 rounded-3xl bg-indigo-500/10 flex items-center justify-center mx-auto mb-6">
        <svg class="w-10 h-10 text-indigo-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
      </div>
      <h3 class="text-xl font-semibold text-white mb-2">Sync IDE Configurations</h3>
      <p class="text-zinc-400 mb-6 max-w-md mx-auto">
        Synchronize your MCP server configurations to all configured IDE clients.
      </p>
      <button
        class="inline-flex items-center gap-2 px-6 py-3 rounded-xl bg-gradient-to-r from-indigo-500 to-purple-500 text-white font-medium hover:from-indigo-600 hover:to-purple-600 transition-all"
        onclick={syncAll}
        disabled={syncing}
      >
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
        Start Sync
      </button>
    </div>
  {/if}
</div>
