<script setup lang="ts">
import { RouterView } from 'vue-router'
import { 
  NConfigProvider, 
  NMessageProvider, 
  NNotificationProvider, 
  NDialogProvider, 
  zhCN, 
  dateZhCN 
} from 'naive-ui'
import type { GlobalThemeOverrides } from 'naive-ui'

// TODO: Integrate with Theme Store for Dark Mode
const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#8b5cf6', 
    primaryColorHover: '#7c3aed',
    primaryColorPressed: '#6d28d9',
  },
  // Add more overrides here to match the design
}
</script>

<template>
  <NConfigProvider 
    :theme-overrides="themeOverrides" 
    :locale="zhCN" 
    :date-locale="dateZhCN"
  >
    <NMessageProvider>
      <NNotificationProvider>
        <NDialogProvider>
          <RouterView />
        </NDialogProvider>
      </NNotificationProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>
