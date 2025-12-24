import { defineStore } from 'pinia';
import { ref } from 'vue';

export const useConfigStore = defineStore('config', () => {
    const appName = ref('Flux Panel');

    return {
        appName
    };
});
