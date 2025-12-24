<script setup lang="ts">
import { h, ref, computed } from 'vue';
import { RouterView, useRoute, useRouter } from 'vue-router';
import { NLayout, NLayoutSider, NLayoutHeader, NLayoutContent, NMenu, NDropdown, NAvatar, NButton, useMessage } from 'naive-ui';
import { isAdmin as checkIsAdmin } from '@/utils/auth';

const collapsed = ref(false);
const route = useRoute();
const router = useRouter();
const message = useMessage();
const isAdmin = checkIsAdmin();

const menuOptions = computed(() => {
  const options = [
    {
      label: '仪表盘',
      key: 'dashboard',
      icon: renderIcon('i-mdi-view-dashboard'),
    },
    // Add other menus based on admin status
  ];
  
  if (isAdmin) {
    options.push(
      { label: '用户管理', key: 'users', icon: renderIcon('i-mdi-account-group') },
      { label: '节点管理', key: 'nodes', icon: renderIcon('i-mdi-server') },
      { label: '隧道管理', key: 'tunnels', icon: renderIcon('i-mdi-tunnel') },
    );
  } else {
    options.push(
      { label: '转发配置', key: 'forwards', icon: renderIcon('i-mdi-share-variant') }, // Example user menu
    );
  }

  options.push(
     { label: '个人设置', key: 'profile', icon: renderIcon('i-mdi-account-cog') },
  );

  return options;
});

function renderIcon(iconClass: string) {
  return () => h('div', { class: iconClass + ' text-lg' });
}

const handleMenuUpdate = (key: string) => {
  router.push({ name: key });
};

const handleLogout = () => {
  localStorage.clear();
  router.push('/login');
  message.success('已退出登录');
};

const userOptions = [
  { label: '修改密码', key: 'change-password' },
  { label: '退出登录', key: 'logout' }
];

const handleUserSelect = (key: string) => {
  if (key === 'logout') {
    handleLogout();
  } else if (key === 'change-password') {
    router.push('/settings/password');
  }
};
</script>

<template>
  <NLayout has-sider class="h-screen">
    <NLayoutSider
      bordered
      collapse-mode="width"
      :collapsed-width="64"
      :width="240"
      :collapsed="collapsed"
      show-trigger
      @collapse="collapsed = true"
      @expand="collapsed = false"
      class="bg-white dark:bg-[#18181c]"
    >
      <div class="h-14 flex items-center justify-center font-bold text-xl border-b border-gray-100 dark:border-gray-800">
        <span v-if="!collapsed">Flux Panel</span>
        <span v-else>FP</span>
      </div>
      <NMenu
        :collapsed="collapsed"
        :collapsed-width="64"
        :collapsed-icon-size="22"
        :options="menuOptions"
        :value="String(route.name)"
        @update:value="handleMenuUpdate"
      />
    </NLayoutSider>

    <NLayout>
      <NLayoutHeader bordered class="h-14 flex items-center justify-between px-6 bg-white dark:bg-[#18181c]">
        <!-- Breadcrumb or Page Title could go here -->
        <h2 class="font-medium text-lg">{{ String(route.meta.title || 'Flux Panel') }}</h2>
        
        <div class="flex items-center gap-4">
           <!-- Theme Switcher placeholder -->
           <NDropdown trigger="click" :options="userOptions" @select="handleUserSelect">
             <div class="flex items-center gap-2 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-800 p-1 rounded-full pr-3 transition-colors">
               <NAvatar round size="small" src="https://ui-avatars.com/api/?name=User" />
               <span class="text-sm font-medium">User</span>
             </div>
           </NDropdown>
        </div>
      </NLayoutHeader>
      
      <NLayoutContent class="bg-gray-50 dark:bg-[#101014] p-0">
        <div class="min-h-full">
           <RouterView v-slot="{ Component }">
             <transition name="fade" mode="out-in">
               <component :is="Component" />
             </transition>
           </RouterView>
        </div>
      </NLayoutContent>
    </NLayout>
  </NLayout>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
