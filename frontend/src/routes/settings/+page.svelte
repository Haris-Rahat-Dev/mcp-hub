<script lang="ts">
  import { toastStore } from '$lib/stores/toast';
  import { onMount } from 'svelte';
  
  let port = $state('4117');
  let host = $state('127.0.0.1');
  let apiKey = $state('');
  let configYaml = $state('');
  let loading = $state(true);
  let saving = $state(false);
  
  onMount(() => {
    loadConfig();
  });
  
  async function loadConfig() {
    loading = true;
    try {
      const resp = await fetch('http://127.0.0.1:4117/api/v1/config');
      if (resp.ok) {
        const data = await resp.json();
        configYaml = data.yaml || '';
      }
    } catch (e) {
      configYaml = '# Failed to load config - is the hub running?';
    }
    loading = false;
  }
  
  async function saveConfig() {
    saving = true;
    try {
      const resp = await fetch('http://127.0.0.1:4117/api/v1/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ yaml: configYaml })
      });
      
      if (resp.ok) {
        toastStore.success('Config saved and hub reloaded');
      } else {
        const err = await resp.json().catch(() => ({}));
        toastStore.error(err.error || 'Failed to save config');
      }
    } catch (e) {
      toastStore.error('Failed to save config');
    }
    saving = false;
  }
</script>

<div class="space-y-6 animate-fade-in">
  <!-- Header -->
  <div>
    <p class="text-zinc-400">Configure mcp-hub</p>
  </div>
  
  <div class="grid grid-cols-2 gap-6">
    <!-- Hub settings -->
    <div class="glass rounded-2xl p-6 border border-white/5">
      <div class="flex items-center gap-3 mb-6">
        <div class="w-10 h-10 rounded-xl bg-indigo-500/10 flex items-center justify-center">
          <svg class="w-5 h-5 text-indigo-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
          </svg>
        </div>
        <div>
          <h3 class="font-medium text-white">Hub Settings</h3>
          <p class="text-xs text-zinc-500">Network configuration</p>
        </div>
      </div>
      
      <div class="space-y-4">
        <div>
          <label class="block text-sm font-medium text-zinc-300 mb-2" for="host-input">Host</label>
          <input
            id="host-input"
            type="text"
            bind:value={host}
            class="w-full bg-white/5 border border-white/10 rounded-xl px-4 py-3 text-white focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-colors"
          />
        </div>
        
        <div>
          <label class="block text-sm font-medium text-zinc-300 mb-2" for="port-input">Port</label>
          <input
            id="port-input"
            type="text"
            bind:value={port}
            class="w-full bg-white/5 border border-white/10 rounded-xl px-4 py-3 text-white focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-colors"
          />
        </div>
        
        <div>
          <label class="block text-sm font-medium text-zinc-300 mb-2" for="apikey-input">API Key</label>
          <input
            id="apikey-input"
            type="password"
            bind:value={apiKey}
            placeholder="Leave empty for localhost-only"
            class="w-full bg-white/5 border border-white/10 rounded-xl px-4 py-3 text-white placeholder-zinc-500 focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-colors"
          />
        </div>
        
        <div class="pt-2">
          <button
            class="w-full px-4 py-3 rounded-xl bg-gradient-to-r from-indigo-500 to-purple-500 text-white font-medium hover:from-indigo-600 hover:to-purple-600 transition-all"
          >
            Save Settings
          </button>
          <p class="text-xs text-zinc-500 text-center mt-2">Host and port changes require restarting the hub.</p>
        </div>
      </div>
    </div>
    
    <!-- Config YAML -->
    <div class="glass rounded-2xl p-6 border border-white/5">
      <div class="flex items-center justify-between mb-6">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-xl bg-purple-500/10 flex items-center justify-center">
            <svg class="w-5 h-5 text-purple-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
            </svg>
          </div>
          <div>
            <h3 class="font-medium text-white">Configuration YAML</h3>
            <p class="text-xs text-zinc-500">Edit raw config</p>
          </div>
        </div>
        <button
          class="px-4 py-2 rounded-xl bg-white/5 text-zinc-400 hover:text-white hover:bg-white/10 transition-colors text-sm"
          onclick={loadConfig}
          disabled={loading}
        >
          Load
        </button>
      </div>
      
      <textarea
        bind:value={configYaml}
        class="w-full h-64 bg-black/30 border border-white/10 rounded-xl px-4 py-3 text-sm text-green-400 font-mono focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-colors resize-none"
        spellcheck="false"
        placeholder="# Click 'Load' to fetch config, or enter YAML here..."
        disabled={loading}
      ></textarea>
      
      <div class="mt-4">
        <button
          class="w-full px-4 py-3 rounded-xl bg-gradient-to-r from-indigo-500 to-purple-500 text-white font-medium hover:from-indigo-600 hover:to-purple-600 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          onclick={saveConfig}
          disabled={saving || loading || !configYaml}
        >
          {saving ? 'Saving...' : 'Save & Reload Hub'}
        </button>
        <p class="text-xs text-zinc-500 text-center mt-2">Changes are hot-reloaded without restart.</p>
      </div>
    </div>
  </div>
  
  <!-- About -->
  <div class="glass rounded-2xl p-6 border border-white/5">
    <div class="flex items-center gap-4">
      <div class="w-14 h-14 rounded-2xl bg-gradient-to-br from-indigo-500 to-purple-600 flex items-center justify-center shadow-lg shadow-indigo-500/25">
        <svg class="w-7 h-7 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2m14-2V6a2 2 0 00-2-2h-4a2 2 0 00-2 2v4m2 0h2" />
        </svg>
      </div>
      <div>
        <h3 class="text-lg font-semibold text-white">mcp-hub v0.1.0</h3>
        <p class="text-sm text-zinc-400">Native macOS app for managing MCP servers</p>
      </div>
    </div>
    <div class="mt-4 pt-4 border-t border-white/5">
      <p class="text-sm text-zinc-500">
        A centralized daemon that runs MCP servers once and shares them across all AI IDEs including Claude Code, Cursor, Windsurf, and more.
      </p>
    </div>
  </div>
</div>
