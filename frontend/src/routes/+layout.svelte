<script lang="ts">
  import '../app.css';
  import Toast from '$lib/components/Toast.svelte';
  import { page } from '$app/stores';
  
  interface Props {
    children: import('svelte').Snippet;
  }
  
  let { children }: Props = $props();
  
  const navItems = [
    { href: '/', label: 'Overview', icon: 'home' },
    { href: '/servers', label: 'Servers', icon: 'server' },
    { href: '/clients', label: 'Clients', icon: 'users' },
    { href: '/sync', label: 'Sync', icon: 'refresh' },
    { href: '/logs', label: 'Logs', icon: 'terminal' },
    { href: '/settings', label: 'Settings', icon: 'settings' }
  ];
  
  const icons: Record<string, string> = {
    home: 'M2.5 12a9.5 9.5 0 1119 0 9.5 9.5 0 01-19 0zM10 12a2 2 0 11-4 0 2 2 0 014 0zM17 12a5 5 0 11-10 0 5 5 0 0110 0z',
    server: 'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2m-14 0a2 2 0 00-2 2v4a2 2 0 002 2m14 0V6a2 2 0 00-2-2h-4a2 2 0 00-2 2v4m2 0h2',
    users: 'M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z',
    refresh: 'M4 4v5h.582m0 0a8.001 8.001 0 0115.356 2M4.582 9H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15',
    terminal: 'M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z',
    settings: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z M15 12a3 3 0 11-6 0 3 3 0 016 0z'
  };
</script>

<div class="flex min-h-screen bg-[#09090b]">
  <!-- Modern Sidebar -->
  <aside class="w-64 glass border-r border-white/5 flex flex-col fixed h-full z-10">
    <!-- Logo -->
    <div class="p-6 border-b border-white/5">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-indigo-500 to-purple-600 flex items-center justify-center shadow-lg shadow-indigo-500/25">
          <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2m14-2V6a2 2 0 00-2-2h-4a2 2 0 00-2 2v4" />
          </svg>
        </div>
        <div>
          <h1 class="font-semibold text-white">mcp-hub</h1>
          <p class="text-xs text-zinc-500">v0.1.0</p>
        </div>
      </div>
    </div>
    
    <!-- Navigation -->
    <nav class="flex-1 p-3 space-y-1">
      {#each navItems as item}
        {@const isActive = $page.url.pathname === item.href}
        <a
          href={item.href}
          class="flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-medium transition-all duration-200 relative group
            {isActive 
              ? 'bg-gradient-to-r from-indigo-500/20 to-purple-500/20 text-white' 
              : 'text-zinc-400 hover:text-white hover:bg-white/5'}"
        >
          {#if isActive}
            <div class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-6 bg-gradient-to-b from-indigo-500 to-purple-500 rounded-r-full"></div>
          {/if}
          <svg class="w-5 h-5 transition-transform duration-200 {isActive ? 'scale-110' : 'group-hover:scale-110'}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d={icons[item.icon]} />
          </svg>
          {item.label}
          {#if isActive}
            <div class="ml-auto w-1.5 h-1.5 rounded-full bg-indigo-500 animate-pulse"></div>
          {/if}
        </a>
      {/each}
    </nav>
    
    <!-- Footer -->
    <div class="p-4 border-t border-white/5">
      <div class="glass rounded-xl p-3">
        <div class="flex items-center gap-2 text-xs text-zinc-500">
          <span class="w-2 h-2 rounded-full bg-green-500 animate-pulse"></span>
          Hub connected
        </div>
      </div>
    </div>
  </aside>
  
  <!-- Main content -->
  <main class="flex-1 ml-64">
    <!-- Top bar -->
    <header class="sticky top-0 z-10 glass border-b border-white/5 px-8 py-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <h2 class="text-lg font-semibold text-white">
            {#if $page.url.pathname === '/'}
              Dashboard
            {:else if $page.url.pathname === '/servers'}
              Servers
            {:else if $page.url.pathname === '/clients'}
              Clients
            {:else if $page.url.pathname === '/sync'}
              Sync
            {:else if $page.url.pathname === '/logs'}
              Logs
            {:else if $page.url.pathname === '/settings'}
              Settings
            {/if}
          </h2>
        </div>
        <div class="flex items-center gap-3">
          <button class="p-2 rounded-lg hover:bg-white/5 text-zinc-400 hover:text-white transition-colors">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
            </svg>
          </button>
        </div>
      </div>
    </header>
    
    <!-- Page content -->
    <div class="p-8">
      {@render children()}
    </div>
  </main>
</div>

<Toast />
