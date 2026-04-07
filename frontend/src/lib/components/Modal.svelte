<script lang="ts">
  import type { Snippet } from 'svelte';
  import { fade, scale } from 'svelte/transition';
  
  interface Props {
    open: boolean;
    title: string;
    onclose: () => void;
    children: Snippet;
    footer?: Snippet;
  }
  
  let { open = $bindable(), title, onclose, children, footer }: Props = $props();
  
  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      onclose();
    }
  }
  
  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      onclose();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center p-4"
    onclick={handleBackdropClick}
    role="dialog"
    aria-modal="true"
    transition:fade={{ duration: 200 }}
  >
    <!-- Backdrop -->
    <div class="absolute inset-0 bg-black/60 backdrop-blur-sm"></div>
    
    <!-- Modal -->
    <div 
      class="relative glass rounded-3xl w-full max-w-lg shadow-2xl border border-white/10 overflow-hidden"
      in:scale={{ start: 0.95, duration: 200 }}
    >
      <!-- Header -->
      <div class="flex items-center justify-between px-6 py-5 border-b border-white/5">
        <h2 class="text-lg font-semibold text-white">{title}</h2>
        <button
          class="p-2 rounded-xl hover:bg-white/5 text-zinc-400 hover:text-white transition-colors"
          onclick={onclose}
        >
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      
      <!-- Content -->
      <div class="px-6 py-5 max-h-[50vh] overflow-y-auto">
        {@render children()}
      </div>
      
      <!-- Footer -->
      {#if footer}
        <div class="px-6 py-4 border-t border-white/5 bg-black/20">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
{/if}
