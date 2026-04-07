<script lang="ts">
  import { toastStore } from '$lib/stores/toast';
  import { fly, fade } from 'svelte/transition';
  
  const icons: Record<string, string> = {
    success: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
    error: 'M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z',
    info: 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
    warn: 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z'
  };
  
  function getColorClasses(type: string): string {
    const map: Record<string, string> = {
      success: 'from-green-500/20 to-emerald-500/20 border-green-500/30',
      error: 'from-red-500/20 to-rose-500/20 border-red-500/30',
      info: 'from-indigo-500/20 to-purple-500/20 border-indigo-500/30',
      warn: 'from-amber-500/20 to-orange-500/20 border-amber-500/30'
    };
    return map[type] || map.info;
  }
</script>

<div class="fixed bottom-6 right-6 z-50 flex flex-col gap-3 max-w-sm">
  {#each $toastStore as toast (toast.id)}
    <div
      class="glass rounded-2xl p-4 flex items-start gap-3 shadow-2xl border backdrop-blur-xl bg-gradient-to-r {getColorClasses(toast.type)} animate-slide-in"
      in:fly={{ x: 100, duration: 300 }}
      out:fade={{ duration: 200 }}
    >
      <div class="shrink-0 mt-0.5">
        {#if toast.type === 'success'}
          <div class="w-8 h-8 rounded-full bg-green-500/20 flex items-center justify-center">
            <svg class="w-4 h-4 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
            </svg>
          </div>
        {:else if toast.type === 'error'}
          <div class="w-8 h-8 rounded-full bg-red-500/20 flex items-center justify-center">
            <svg class="w-4 h-4 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </div>
        {:else if toast.type === 'warn'}
          <div class="w-8 h-8 rounded-full bg-amber-500/20 flex items-center justify-center">
            <svg class="w-4 h-4 text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
        {:else}
          <div class="w-8 h-8 rounded-full bg-indigo-500/20 flex items-center justify-center">
            <svg class="w-4 h-4 text-indigo-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
        {/if}
      </div>
      
      <p class="text-sm text-white flex-1 leading-relaxed">{toast.message}</p>
      
      <button
        class="shrink-0 p-1 rounded-lg hover:bg-white/10 text-zinc-400 hover:text-white transition-colors"
        onclick={() => toastStore.remove(toast.id)}
      >
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  {/each}
</div>
