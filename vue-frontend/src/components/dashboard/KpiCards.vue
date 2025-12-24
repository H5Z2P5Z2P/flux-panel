<script setup lang="ts">
import { computed } from 'vue';
import { NCard, NGrid, NGridItem, NProgress } from 'naive-ui';
import { formatFlow, formatNumber, formatResetTime } from '@/utils/format';
import type { User } from '@/types';

const props = defineProps<{
  userInfo: User;
  usedFlow: number;
  forwardCount: number;
}>();

const flowPercentage = computed(() => {
  if (props.userInfo.flow === 99999) return 0;
  const totalLimit = (props.userInfo.flow || 0) * 1024 * 1024 * 1024;
  return totalLimit > 0 ? Math.min((props.usedFlow / totalLimit) * 100, 100) : 0;
});

const forwardPercentage = computed(() => {
  if (props.userInfo.num === 99999) return 0;
  const totalLimit = props.userInfo.num || 0;
  return totalLimit > 0 ? Math.min((props.forwardCount / totalLimit) * 100, 100) : 0;
});

const getStatusColor = (percentage: number) => {
  if (percentage >= 90) return '#ef4444';
  if (percentage >= 70) return '#f97316';
  return '#3b82f6';
};
</script>

<template>
  <NGrid x-gap="12" y-gap="12" cols="2 s:2 m:4 l:4" responsive="screen">
    <NGridItem>
      <NCard hoverable size="small" class="h-full">
        <div class="text-xs text-gray-500 mb-1">总流量</div>
        <div class="text-xl font-bold flex items-center justify-between">
          {{ formatFlow(userInfo.flow, 'gb') }}
          <div class="p-2 bg-blue-100 dark:bg-blue-900/30 rounded-lg">
            <div class="i-mdi-network text-blue-600 dark:text-blue-400" />
          </div>
        </div>
      </NCard>
    </NGridItem>

    <NGridItem>
      <NCard hoverable size="small" class="h-full">
        <div class="text-xs text-gray-500 mb-1">已用流量</div>
        <div class="text-xl font-bold mb-2">{{ formatFlow(usedFlow) }}</div>
        <NProgress 
          type="line" 
          :percentage="flowPercentage" 
          :color="getStatusColor(flowPercentage)"
          :show-indicator="false"
          :height="6"
        />
        <div class="flex justify-between mt-1 text-xs text-gray-400">
          <span>{{ userInfo.flow === 99999 ? '无限制' : `${flowPercentage.toFixed(1)}%` }}</span>
          <span v-if="userInfo.flowResetTime">{{ formatResetTime(userInfo.flowResetTime) }}</span>
        </div>
      </NCard>
    </NGridItem>

    <NGridItem>
      <NCard hoverable size="small" class="h-full">
        <div class="text-xs text-gray-500 mb-1">转发配额</div>
        <div class="text-xl font-bold flex items-center justify-between">
          {{ formatNumber(userInfo.num) }}
          <div class="p-2 bg-purple-100 dark:bg-purple-900/30 rounded-lg">
            <div class="i-mdi-share-variant text-purple-600 dark:text-purple-400" />
          </div>
        </div>
      </NCard>
    </NGridItem>

    <NGridItem>
      <NCard hoverable size="small" class="h-full">
        <div class="text-xs text-gray-500 mb-1">已用转发</div>
        <div class="text-xl font-bold mb-2">{{ forwardCount }}</div>
        <NProgress 
          type="line" 
          :percentage="forwardPercentage" 
          :color="getStatusColor(forwardPercentage)"
          :show-indicator="false"
          :height="6"
        />
        <div class="flex justify-between mt-1 text-xs text-gray-400">
          <span>{{ userInfo.num === 99999 ? '无限制' : `${forwardPercentage.toFixed(1)}%` }}</span>
        </div>
      </NCard>
    </NGridItem>
  </NGrid>
</template>
