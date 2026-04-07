<script lang="ts">
  import { hubStore } from '$lib/stores/hub';
  import { toastStore } from '$lib/stores/toast';
  import ServerCard from '$lib/components/ServerCard.svelte';
  import Modal from '$lib/components/Modal.svelte';
  
  let showAddModal = $state(false);
  let newServer = $state({ name: '', mode: 'shared', command: '', url: '', lazy: true });
  let saving = $state(false);
  let loading = $state(true);
  
  async function loadServers() {
    try {
      const resp = await fetch('http://127.0.0.1:4117/api/v1/servers');
      if (resp.ok) {
        const data = await resp.json();
        hubStore.update(s => ({ ...s, servers: data.servers || [] }));
      }
    } catch (e) {
      console.error('Failed to load servers:', e);
    }
    loading = false;
  }
  
  async function addServer() {
    if (!newServer.name) {
      toastStore.error('Server name is required');
      return;
    }
    if (newServer.mode === 'passthrough' && !newServer.url) {
      toastStore.error('URL is required for passthrough mode');
      return;
    }
    if (newServer.mode !== 'passthrough' && !newServer.command) {
      toastStore.error('Command is required for shared/isolated modes');
      return;
    }
    
    saving = true;
    try {
      const resp = await fetch('http://127.0.0.1:4117/api/v1/servers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: newServer.name,
          mode: newServer.mode,
          command: newServer.command,
          url: newServer.url,
          lazy: newServer.lazy
        })
      });
      
      if (resp.ok) {
        toastStore.success(`Server "${newServer.name}" added`);
        showAddModal = false;
        newServer = { name: '', mode: 'shared', command: '', url: '', lazy: true };
        await loadServers();
      } else {
        const err = await resp.json().catch(() => ({}));
        toastStore.error(err.error || 'Failed to add server');
      }
    } catch (e) {
      toastStore.error('Failed to add server');
    }
    saving = false;
  }
  
  async function removeServer(name: string) {
    if (!confirm(`Remove server "${name}"?`)) return;
    
    try {
      const resp = await fetch(`http://127.0.0.1:4117/api/v1/servers/${encodeURIComponent(name)}`, {
        method: 'DELETE'
      });
      
      if (resp.ok) {
        toastStore.success(`Server "${name}" removed`);
        await loadServers();
      } else {
        toastStore.error('Failed to remove server');
      }
    } catch (e) {
      toastStore.error('Failed to remove server');
    }
  }
  
  // Load servers on mount
  loadServers();
</script>

<div class="space-y-6 animate-fade-in">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div>
      <p class="text-zinc-400">Manage your MCP servers</p>
    </div>
    <button
      class="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-gradient-to-r from-indigo-500 to-purple-500 text-white font-medium hover:from-indigo-600 hover:to-purple-600 transition-all shadow-lg shadow-indigo-500/25"
      onclick={() => showAddModal = true}
    >
      <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
      </svg>
      Add Server
    </button>
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
      <h3 class="text-xl font-semibold text-white mb-2">No servers yet</h3>
      <p class="text-zinc-400 mb-6">Add your first MCP server to get started</p>
      <button 
        class="inline-flex items-center gap-2 px-6 py-3 rounded-xl bg-gradient-to-r from-indigo-500 to-purple-500 text-white font-medium hover:from-indigo-600 hover:to-purple-600 transition-all"
        onclick={() => showAddModal = true}
      >
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        Add Your First Server
      </button>
    </div>
  {:else}
    <!-- Server cards grid -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {#each $hubStore.servers as server, i}
        <div class="animate-fade-in" style="animation-delay: {i * 50}ms">
          <ServerCard {server} onremove={removeServer} />
        </div>
      {/each}
    </div>
  {/if}
</div>

<Modal bind:open={showAddModal} title="Add New Server" onclose={() => showAddModal = false}>
  <div class="space-y-5">
    <div>
      <label class="block text-sm font-medium text-zinc-300 mb-2" for="srv-name">Server Name</label>
      <input
        id="srv-name"
        type="text"
        bind:value={newServer.name}
        placeholder="e.g. context7, fetch, puppeteer"
        class="w-full bg-white/5 border border-white/10 rounded-xl px-4 py-3 text-white placeholder-zinc-500 focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-colors"
      />
    </div>
    
    <div>
      <label class="block text-sm font-medium text-zinc-300 mb-2" for="srv-mode">Mode</label>
      <select
        id="srv-mode"
        bind:value={newServer.mode}
        class="w-full appearance-none bg-white/5 border border-white/10 rounded-xl px-4 py-3 text-white focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-colors cursor-pointer"
        style="background-image: url('data:image/svg+xml;charset=UTF-8,%3csvg xmlns=%27http://www.w3.org/2000/svg%27 viewBox=%270 0 24 24%27 fill=%27none%27 stroke=%27%23a1a1aa%27 stroke-width=%272%27 stroke-linecap=%27round%27 stroke-linejoin=%27round%27%3e%3cpolyline points=%276 9 12 15 18 9%27%3e%3c/polyline%3e%3c/svg%3e'); background-repeat: no-repeat; background-position: right 1rem center; background-size: 1rem;"
      >
        <option value="shared">Shared — Single process, all clients</option>
        <option value="isolated">Isolated — One process per client</option>
        <option value="passthrough">Passthrough — Proxy to remote URL</option>
      </select>
    </div>
    
    {#if newServer.mode === 'passthrough'}
      <div>
        <label class="block text-sm font-medium text-zinc-300 mb-2" for="srv-url">Remote URL</label>
        <input
          id="srv-url"
          type="text"
          bind:value={newServer.url}
          placeholder="https://mcp.example.com/mcp"
          class="w-full bg-white/5 border border-white/10 rounded-xl px-4 py-3 text-white placeholder-zinc-500 focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-colors"
        />
      </div>
    {:else}
      <div>
        <label class="block text-sm font-medium text-zinc-300 mb-2" for="srv-cmd">Command</label>
        <input
          id="srv-cmd"
          type="text"
          bind:value={newServer.command}
          placeholder="npx, node, python, etc."
          class="w-full bg-white/5 border border-white/10 rounded-xl px-4 py-3 text-white placeholder-zinc-500 focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-colors"
        />
      </div>
    {/if}
    
    <div class="flex items-center gap-3">
      <input type="checkbox" id="srv-lazy" bind:checked={newServer.lazy} class="w-4 h-4 rounded border-white/20 bg-white/5 text-indigo-500 focus:ring-indigo-500 focus:ring-offset-0" />
      <label for="srv-lazy" class="text-sm text-zinc-300">Lazy start — Start server on first request</label>
    </div>
  </div>
  
  {#snippet footer()}
    <div class="flex justify-end gap-3">
      <button
        class="px-5 py-2.5 rounded-xl text-zinc-400 hover:text-white hover:bg-white/5 transition-colors"
        onclick={() => showAddModal = false}
      >
        Cancel
      </button>
      <button
        class="px-5 py-2.5 rounded-xl bg-gradient-to-r from-indigo-500 to-purple-500 text-white font-medium hover:from-indigo-600 hover:to-purple-600 transition-all disabled:opacity-50"
        onclick={addServer}
        disabled={saving}
      >
        {saving ? 'Adding...' : 'Add Server'}
      </button>
    </div>
  {/snippet}
</Modal>
