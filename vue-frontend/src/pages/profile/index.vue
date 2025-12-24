<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { NCard, NAvatar, NGrid, NGridItem, NButton, useMessage } from 'naive-ui';
import { useRouter } from 'vue-router';
// import { safeLogout } from '@/utils/auth'; // Ensure accessible
import { siteConfig } from '@/config/site'; // Mock if not exists
// import { isWebViewFunc } from '@/utils/panel';

const router = useRouter();
const message = useMessage();

const username = ref(localStorage.getItem('name') || 'Admin');
const isAdmin = ref(localStorage.getItem('admin') === 'true' || localStorage.getItem('role_id') === '0');

const menuItems = [
    {
        label: '限速管理',
        path: '/limits',
        icon: 'i-mdi-speedometer',
        color: 'text-orange-500',
        bg: 'bg-orange-100 dark:bg-orange-900/20',
        show: isAdmin.value
    },
    {
        label: '用户管理',
        path: '/users',
        icon: 'i-mdi-account-group',
        color: 'text-blue-500',
        bg: 'bg-blue-100 dark:bg-blue-900/20',
        show: isAdmin.value
    },
    {
        label: '网站配置',
        path: '/config',
        icon: 'i-mdi-cog',
        color: 'text-purple-500',
        bg: 'bg-purple-100 dark:bg-purple-900/20',
        show: isAdmin.value
    },
    {
        label: '修改密码',
        path: '/settings/password',
        icon: 'i-mdi-lock-reset',
        color: 'text-blue-600',
        bg: 'bg-blue-50 dark:bg-blue-900/10',
        show: true
    },
    {
        label: '面板设置',
        path: '/settings',
        icon: 'i-mdi-server-network',
        color: 'text-green-600',
        bg: 'bg-green-50 dark:bg-green-900/10',
        show: true
    }
];

const handleLogout = () => {
    localStorage.clear();
    router.push('/login');
    message.success('已退出登录');
};

</script>

<template>
  <div class="p-6 h-full flex flex-col">
      <!-- Profile Card -->
      <NCard class="mb-6 shadow-sm border-0 bg-gradient-to-r from-primary-500/10 to-primary-100/50 dark:from-primary-900/20 dark:to-transparent">
          <div class="flex items-center gap-4">
              <div class="w-16 h-16 rounded-full bg-primary-100 dark:bg-primary-800 flex items-center justify-center text-primary-600 dark:text-primary-200 text-2xl">
                  <div class="i-mdi-account" />
              </div>
              <div>
                  <h2 class="text-xl font-bold">{{ username }}</h2>
                  <div class="flex items-center gap-2 mt-1">
                      <span class="px-2 py-0.5 rounded text-xs" :class="isAdmin ? 'bg-primary-100 text-primary-700' : 'bg-gray-100 text-gray-600'">
                          {{ isAdmin ? '管理员' : '普通用户' }}
                      </span>
                  </div>
              </div>
          </div>
      </NCard>

      <!-- Grid Menu -->
      <NCard title="功能中心" class="flex-1 shadow-sm">
          <div class="grid grid-cols-3 gap-4">
              <template v-for="item in menuItems" :key="item.path">
                  <div 
                    v-if="item.show"
                    class="flex flex-col items-center justify-center p-4 rounded-xl cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
                    @click="router.push(item.path)"
                  >
                      <div class="w-12 h-12 rounded-full flex items-center justify-center mb-2 text-2xl" :class="[item.bg, item.color]">
                          <div :class="item.icon" />
                      </div>
                      <span class="text-xs text-gray-600 dark:text-gray-300">{{ item.label }}</span>
                  </div>
              </template>
              
              <!-- Logout -->
              <div 
                class="flex flex-col items-center justify-center p-4 rounded-xl cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
                @click="handleLogout"
              >
                  <div class="w-12 h-12 rounded-full bg-red-100 dark:bg-red-900/20 text-red-500 flex items-center justify-center mb-2 text-2xl">
                      <div class="i-mdi-logout" />
                  </div>
                  <span class="text-xs text-gray-600 dark:text-gray-300">退出登录</span>
              </div>
          </div>
      </NCard>

      <div class="text-center py-6 text-xs text-gray-400">
          Powered by flux-panel
      </div>
  </div>
</template>
