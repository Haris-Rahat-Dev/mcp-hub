import { writable } from 'svelte/store';
import type { Toast } from '$lib/types';

function createToastStore() {
  const { subscribe, update } = writable<Toast[]>([]);

  function add(message: string, type: Toast['type'] = 'info') {
    const id = Math.random().toString(36).slice(2);
    update(toasts => [...toasts, { id, type, message }]);
    
    setTimeout(() => {
      remove(id);
    }, 4000);
  }

  function remove(id: string) {
    update(toasts => toasts.filter(t => t.id !== id));
  }

  return {
    subscribe,
    success: (msg: string) => add(msg, 'success'),
    error: (msg: string) => add(msg, 'error'),
    info: (msg: string) => add(msg, 'info'),
    warn: (msg: string) => add(msg, 'warn'),
    remove
  };
}

export const toastStore = createToastStore();
