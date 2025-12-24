<script setup lang="ts">
import { onMounted, computed } from 'vue';
import { useDashboardData } from '@/composables/dashboard/useDashboardData';
import { useExpirationNotify } from '@/composables/dashboard/useExpirationNotify';
import { isAdmin as checkIsAdmin } from '@/utils/auth';

import DashboardHeader from '@/components/dashboard/DashboardHeader.vue';
import KpiCards from '@/components/dashboard/KpiCards.vue';
import FlowChart from '@/components/dashboard/FlowChart.vue';
import TunnelPermissions from '@/components/dashboard/TunnelPermissions.vue';
import ForwardGroups from '@/components/dashboard/ForwardGroups.vue';
import { NCard, NDivider } from 'naive-ui';

// 1. Data Fetching
const { loading, userInfo, userTunnels, forwardList, statisticsFlows, loadData } = useDashboardData();

// 2. Logic: Admin check
const isAdmin = checkIsAdmin();

// 3. Logic: Total Used Flow (Computed)
const usedFlow = computed(() => (userInfo.value.inFlow || 0) + (userInfo.value.outFlow || 0));

// 4. Logic: Notification
const { checkAndNotify } = useExpirationNotify();

onMounted(async () => {
  // Clear "Last Page" state if needed
  localStorage.setItem('e', '/dashboard');

  await loadData();
  
  if (userInfo.value && userTunnels.value) {
    checkAndNotify(userInfo.value, userTunnels.value);
  }
});
</script>

<template>
  <div class="max-w-7xl mx-auto p-4 md:p-6 lg:p-8">
    <!-- Header -->
    <DashboardHeader 
      :exp-time="userInfo?.expTime" 
      :loading="loading"
      @refresh="loadData"
    />

    <!-- KPI Cards -->
    <KpiCards 
      :user-info="userInfo"
      :used-flow="usedFlow"
      :forward-count="forwardList.length"
      class="mb-6"
    />

    <!-- Charts -->
    <NCard title="24小时流量统计" class="mb-6 shadow-sm" :bordered="false">
      <template #header-extra>
        <div class="text-xs text-gray-400">实时更新</div>
      </template>
      <FlowChart :data="statisticsFlows" />
    </NCard>

    <!-- Admin Hidden Sections -->
    <template v-if="!isAdmin">
      <NDivider title-placement="left" class="text-gray-400">隧道权限</NDivider>
      <TunnelPermissions :tunnels="userTunnels" class="mb-8" />
    </template>

    <NDivider title-placement="left" class="text-gray-400">转发配置</NDivider>
    <ForwardGroups :forward-list="forwardList" />
  </div>
</template>
