<script setup lang="ts">
import { RouterView } from 'vue-router';
// Simple layout for Guest: just a container, maybe a simple header or logo if needed.
// For now, it's just a wrapper.
</script>

<template>
  <div class="min-h-screen bg-gray-50 dark:bg-[#101014] text-gray-900 dark:text-gray-100 transition-colors duration-300">
    <!-- Header placeholder -->
     <header class="h-14 border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-[#18181c] flex items-center px-6">
        <div class="font-bold text-lg">Flux Panel</div>
     </header>
    <main>
      <RouterView />
    </main>
  </div>
</template>
