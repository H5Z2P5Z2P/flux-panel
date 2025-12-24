<script setup lang="ts">
import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { LineChart } from 'echarts/charts';
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components';
import VChart from 'vue-echarts';
import { ref, computed } from 'vue';
import { useOsTheme } from 'naive-ui';
import { processFlowChartData } from '@/utils/flowChart';

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent]);

const props = defineProps<{
  data: any[];
}>();

const osTheme = useOsTheme();
const isDark = computed(() => osTheme.value === 'dark');

const chartData = computed(() => processFlowChartData(props.data));

const option = computed(() => {
  const textColor = isDark.value ? '#a3a3a3' : '#666';
  const lineColor = isDark.value ? '#333' : '#eee';
  
  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      formatter: (params: any) => {
        const item = params[0];
        const dataItem = chartData.value[item.dataIndex];
        return `
          <div class="px-2 py-1">
            <div class="font-medium mb-1">${item.axisValue}</div>
            <div class="text-primary">流量: ${dataItem ? dataItem.formattedFlow : '0 B'}</div>
          </div>
        `;
      },
      backgroundColor: isDark.value ? '#333' : '#fff',
      borderColor: isDark.value ? '#444' : '#ddd',
      textStyle: { color: isDark.value ? '#eee' : '#333' }
    },
    grid: {
      left: '2%',
      right: '2%',
      bottom: '5%',
      top: '10%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: chartData.value.map(i => i.time),
      boundaryGap: false,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: textColor }
    },
    yAxis: {
      type: 'value',
      splitLine: { 
        lineStyle: { type: 'dashed', color: lineColor } 
      },
      axisLabel: {
        color: textColor,
        formatter: (val: number) => {
          if (val === 0) return '0';
          if (val < 1024) return val + 'B';
          if (val < 1024 * 1024) return (val / 1024).toFixed(0) + 'K';
          if (val < 1024 * 1024 * 1024) return (val / (1024 * 1024)).toFixed(0) + 'M';
          return (val / (1024 * 1024 * 1024)).toFixed(0) + 'G';
        }
      }
    },
    series: [
      {
        data: chartData.value.map(i => i.value),
        type: 'line',
        smooth: true,
        symbol: 'none',
        areaStyle: {
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(139, 92, 246, 0.3)' },
              { offset: 1, color: 'rgba(139, 92, 246, 0.05)' }
            ]
          }
        },
        itemStyle: { color: '#8b5cf6' },
        lineStyle: { width: 3 }
      }
    ]
  };
});
</script>

<template>
  <div class="h-64 md:h-80 w-full">
    <VChart :option="option" autoresize />
  </div>
</template>
