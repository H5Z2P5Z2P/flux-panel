<script setup lang="ts">
import { onMounted } from 'vue';
import { useRoute } from 'vue-router';
import { useDashboardData } from '@/composables/dashboard/useDashboardData';
import KpiCards from '@/components/dashboard/KpiCards.vue';
import FlowChart from '@/components/dashboard/FlowChart.vue';
import TunnelPermissions from '@/components/dashboard/TunnelPermissions.vue';
import ForwardGroups from '@/components/dashboard/ForwardGroups.vue';
import { NCard, NDivider } from 'naive-ui';

const route = useRoute();
const token = route.query.token as string;

// Pass isGuest=true
const { loading, userInfo, userTunnels, forwardList, statisticsFlows, loadData } = 
  useDashboardData(true, token);

onMounted(() => {
  if (token) loadData();
});
</script>

<template>
  <div class="max-w-7xl mx-auto p-4 md:p-6 lg:p-8">
    <div class="mb-6">
      <h1 class="text-2xl font-bold dark:text-white">访客仪表盘</h1>
      <p class="text-sm text-gray-500">仅供查看转发使用情况</p>
    </div>
    
    <div v-if="loading" class="text-center py-20 text-gray-400">
      <div class="i-mdi-loading animate-spin text-3xl mb-2" />
      <div>加载中...</div>
    </div>
    
    <template v-else>
      <KpiCards 
        :user-info="userInfo"
        :used-flow="(userInfo.inFlow||0) + (userInfo.outFlow||0)"
        :forward-count="forwardList.length"
        class="mb-6"
      />

      <NCard title="24小时流量统计" class="mb-6 shadow-sm" :bordered="false">
        <FlowChart :data="statisticsFlows" />
      </NCard>
      
      <div v-if="userTunnels.length > 0">
         <NDivider title-placement="left" class="text-gray-400">隧道权限</NDivider>
         <TunnelPermissions :tunnels="userTunnels" class="mb-8" />
      </div>

      <NDivider title-placement="left" class="text-gray-400">转发配置</NDivider>
      <ForwardGroups :forward-list="forwardList" />
    </template>
  </div>
</template>
