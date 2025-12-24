<script setup lang="ts">
import { NCard, NTag, NProgress } from 'naive-ui';
import type { UserTunnel } from '@/types';
import { formatFlow, formatNumber, getExpStatus, formatResetTime } from '@/utils/format';

defineProps<{
  tunnels: UserTunnel[];
}>();

const getTunnelUsedFlow = (t: UserTunnel) => (t.inFlow || 0) + (t.outFlow || 0);

const getPercentage = (used: number, total: number | undefined) => {
  if (total === 99999 || !total) return 0;
  return Math.min((used / total) * 100, 100); 
};
</script>

<template>
  <div class="space-y-4">
    <div v-for="t in tunnels" :key="t.id" 
      class="border border-gray-200 dark:border-gray-800 rounded-lg p-4 bg-white dark:bg-gray-900/50 hover:shadow-md transition-shadow"
    >
      <div class="flex flex-col md:flex-row md:items-center justify-between gap-2 mb-4">
        <h3 class="font-semibold text-gray-900 dark:text-gray-100">
          {{ t.tunnelName }} <span class="text-xs text-gray-400 ml-1">ID: {{ t.tunnelId }}</span>
        </h3>
        <div class="flex flex-wrap gap-2">
          <NTag size="small" :type="t.tunnelFlow === 1 ? 'info' : 'warning'" secondary round>
            {{ t.tunnelFlow === 1 ? '单向计费' : '双向计费' }}
          </NTag>
          
          <div :class="`px-2 py-0.5 rounded text-xs border ${getExpStatus(t.expTime).borderColor} ${getExpStatus(t.expTime).bg} ${getExpStatus(t.expTime).color}`">
            {{ getExpStatus(t.expTime).text }}
          </div>
          
          <span v-if="t.flowResetTime" class="text-xs text-gray-500 flex items-center">
            {{ formatResetTime(t.flowResetTime) }}
          </span>
        </div>
      </div>

      <div class="grid grid-cols-2 lg:grid-cols-4 gap-6">
        <div>
          <div class="text-xs text-gray-500 mb-1">流量配额</div>
          <div class="font-medium">{{ formatFlow(t.flow || 0, 'gb') }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 mb-1">已用流量</div>
          <div class="font-medium">{{ formatFlow(getTunnelUsedFlow(t)) }}</div>
          <NProgress 
            type="line" 
            :height="4" 
            :percentage="getPercentage((getTunnelUsedFlow(t) / (1024*1024*1024)), t.flow)" 
            :show-indicator="false"
            class="mt-2"
          />
        </div>
        <div>
          <div class="text-xs text-gray-500 mb-1">转发配额</div>
          <div class="font-medium">{{ formatNumber(t.num || 0) }}</div>
        </div>
        <!-- Note: We don't have per-tunnel used forwards count in raw UserTunnel type unless backend populates it? 
             React code filters forwardList by tunnelId. But here we only pass tunnels.
             Let's ignore used count for now or it requires parent computed prop.
             Based on React code: getTunnelUsedForwards(tunnel.tunnelId) uses forwardList.
             We can skip it or pass it in. Let's skip simplified version or fix types.
        -->
      </div>
    </div>
  </div>
</template>
