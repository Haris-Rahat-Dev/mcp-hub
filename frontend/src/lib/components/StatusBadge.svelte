<script lang="ts">
  interface Props {
    status: 'running' | 'idle' | 'error' | 'stopped' | 'starting' | 'restarting';
    showLabel?: boolean;
    size?: 'sm' | 'md' | 'lg';
  }
  
  let { status, showLabel = true, size = 'md' }: Props = $props();
  
  const statusConfig = {
    running: { 
      color: 'bg-green-500', 
      glow: 'shadow-green-500/50', 
      label: 'Running',
      pulse: true
    },
    idle: { 
      color: 'bg-amber-500', 
      glow: 'shadow-amber-500/50', 
      label: 'Idle',
      pulse: false
    },
    error: { 
      color: 'bg-red-500', 
      glow: 'shadow-red-500/50', 
      label: 'Error',
      pulse: true
    },
    stopped: { 
      color: 'bg-zinc-500', 
      glow: '', 
      label: 'Stopped',
      pulse: false
    },
    starting: { 
      color: 'bg-indigo-500', 
      glow: 'shadow-indigo-500/50', 
      label: 'Starting',
      pulse: true
    },
    restarting: { 
      color: 'bg-amber-500', 
      glow: 'shadow-amber-500/50', 
      label: 'Restarting',
      pulse: true
    }
  };
  
  const config = $derived(statusConfig[status] || statusConfig.idle);
  const sizeClasses = $derived({
    sm: 'w-2 h-2',
    md: 'w-2.5 h-2.5',
    lg: 'w-3 h-3'
  }[size]);
</script>

<span class="flex items-center gap-2">
  <span class="relative flex">
    <span class="{sizeClasses} rounded-full {config.color} {config.glow} shadow-lg"></span>
    {#if config.pulse}
      <span class="{sizeClasses} rounded-full {config.color} absolute top-0 left-0 animate-ping opacity-60"></span>
    {/if}
  </span>
  {#if showLabel}
    <span class="text-sm font-medium text-zinc-300">{config.label}</span>
  {/if}
</span>
