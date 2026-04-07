<script lang="ts">
  import { onMount } from 'svelte';
  import type { LogEntry } from '$lib/api';
  
  let logs = $state<LogEntry[]>([]);
  let filter = $state('all');
  let autoScroll = $state(true);
  let searchQuery = $state('');
  
  onMount(() => {
    // Simulated logs for demo
    logs = [
      { time: new Date().toISOString(), level: 'info', server: 'context7', message: 'Server started successfully' },
      { time: new Date(Date.now() - 60000).toISOString(), level: 'info', server: 'fetch', message: 'Accepted client connection from Cursor' },
      { time: new Date(Date.now() - 120000).toISOString(), level: 'warn', server: 'puppeteer', message: 'High memory usage: 312 MB' },
      { time: new Date(Date.now() - 180000).toISOString(), level: 'error', server: 'postgres', message: 'Connection refused to localhost:5432' },
      { time: new Date(Date.now() - 240000).toISOString(), level: 'info', server: 'memory', message: 'Initialized knowledge graph with 1.2k entries' }
    ];
  });
  
  const filteredLogs = $derived(
    logs.filter(l => {
      if (filter !== 'all' && l.level !== filter) return false;
      if (searchQuery && !l.message.toLowerCase().includes(searchQuery.toLowerCase()) && !l.server.toLowerCase().includes(searchQuery.toLowerCase())) return false;
      return true;
    })
  );
  
  const levelColors: Record<string, { bg: string; text: string; border: string }> = {
    debug: { bg: 'bg-zinc-500/10', text: 'text-zinc-400', border: 'border-zinc-500/20' },
    info: { bg: 'bg-indigo-500/10', text: 'text-indigo-400', border: 'border-indigo-500/20' },
    warn: { bg: 'bg-amber-500/10', text: 'text-amber-400', border: 'border-amber-500/20' },
    error: { bg: 'bg-red-500/10', text: 'text-red-400', border: 'border-red-500/20' }
  };
  
  function formatTime(iso: string): string {
    const d = new Date(iso);
    return d.toLocaleTimeString('en-US', { hour12: false });
  }
</script>

<div class="space-y-6 animate-fade-in">
  <!-- Header with controls -->
  <div class="flex items-center justify-between">
    <div>
      <p class="text-zinc-400">Real-time server logs</p>
    </div>
    <div class="flex items-center gap-3">
      <!-- Search -->
      <div class="relative">
        <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Search logs..."
          class="w-64 bg-white/5 border border-white/10 rounded-xl pl-10 pr-4 py-2 text-sm text-white placeholder-zinc-500 focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-colors"
        />
      </div>
      
      <!-- Level filter -->
      <select
        bind:value={filter}
        class="w-32 appearance-none bg-white/5 border border-white/10 rounded-xl px-4 py-2 text-sm text-white focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-colors cursor-pointer"
        style="background-image: url('data:image/svg+xml;charset=UTF-8,%3csvg xmlns=%27http://www.w3.org/2000/svg%27 viewBox=%270 0 24 24%27 fill=%27none%27 stroke=%27%23a1a1aa%27 stroke-width=%272%27 stroke-linecap=%27round%27 stroke-linejoin=%27round%27%3e%3cpolyline points=%276 9 12 15 18 9%27%3e%3c/polyline%3e%3c/svg%3e'); background-repeat: no-repeat; background-position: right 0.75rem center; background-size: 1rem;"
      >
        <option value="all">All levels</option>
        <option value="debug">Debug</option>
        <option value="info">Info</option>
        <option value="warn">Warning</option>
        <option value="error">Error</option>
      </select>
      
      <!-- Auto-scroll toggle -->
      <label class="flex items-center gap-2 cursor-pointer">
        <span class="text-xs text-zinc-500">Auto-scroll</span>
        <div class="relative">
          <input type="checkbox" bind:checked={autoScroll} class="sr-only peer" />
          <div class="w-9 h-5 bg-white/10 rounded-full peer peer-checked:bg-indigo-500/30 transition-colors"></div>
          <div class="absolute left-0.5 top-0.5 w-4 h-4 bg-zinc-500 rounded-full peer-checked:translate-x-4 peer-checked:bg-indigo-500 transition-all"></div>
        </div>
      </label>
    </div>
  </div>
  
  <!-- Log viewer -->
  <div class="glass rounded-2xl border border-white/5 overflow-hidden">
    {#if filteredLogs.length === 0}
      <div class="p-12 text-center">
        <svg class="w-12 h-12 text-zinc-600 mx-auto mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
        </svg>
        <p class="text-zinc-500">No logs to display</p>
      </div>
    {:else}
      <div class="max-h-[60vh] overflow-y-auto font-mono text-sm">
        {#each filteredLogs as log}
          {@const colors = levelColors[log.level] || levelColors.info}
          <div class="flex items-start gap-4 px-5 py-3 hover:bg-white/[0.02] transition-colors border-b border-white/5 last:border-0">
            <span class="text-zinc-600 shrink-0 w-20">{formatTime(log.time)}</span>
            <span class="shrink-0 w-16">
              <span class="text-xs px-2 py-0.5 rounded {colors.bg} {colors.text} border {colors.border} uppercase">
                {log.level}
              </span>
            </span>
            <span class="shrink-0 w-24 text-indigo-400 truncate">{log.server}</span>
            <span class="text-zinc-300 flex-1 break-all">{log.message}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>
  
  <!-- Stats bar -->
  <div class="flex items-center gap-6 text-sm">
    <span class="text-zinc-500">
      Showing <span class="text-white">{filteredLogs.length}</span> of <span class="text-white">{logs.length}</span> entries
    </span>
    <div class="flex items-center gap-4">
      <span class="flex items-center gap-1.5">
        <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
        <span class="text-zinc-500">Info ({logs.filter(l => l.level === 'info').length})</span>
      </span>
      <span class="flex items-center gap-1.5">
        <span class="w-2 h-2 rounded-full bg-amber-500"></span>
        <span class="text-zinc-500">Warn ({logs.filter(l => l.level === 'warn').length})</span>
      </span>
      <span class="flex items-center gap-1.5">
        <span class="w-2 h-2 rounded-full bg-red-500"></span>
        <span class="text-zinc-500">Error ({logs.filter(l => l.level === 'error').length})</span>
      </span>
    </div>
  </div>
</div>
