<script setup lang="ts">
import { ref, onMounted, onUnmounted, reactive } from 'vue';
import { useRouter } from 'vue-router';
import { NCard, NInput, NButton, useMessage, useOsTheme } from 'naive-ui';
import { login, checkCaptcha } from '@/api';
import type { LoginData } from '@/api';
import { loadTajs } from '@/plugins/captcha';
import { isWebViewFunc } from '@/utils/panel';
import bgImage from '@/assets/images/bg.jpg';
import axios from 'axios';

const router = useRouter();
const message = useMessage();
const osTheme = useOsTheme();

const form = reactive<LoginData & { captchaId: string }>({
  username: '',
  password: '',
  captchaId: ''
});

const loading = ref(false);
const showCaptcha = ref(false);
const isWebView = ref(false);
const captchaContainerRef = ref<HTMLDivElement | null>(null);
const tacInstance = ref<any>(null);

const validateForm = () => {
    if (!form.username) {
        message.warning('请输入用户名');
        return false;
    }
    if (!form.password) {
        message.warning('请输入密码');
        return false;
    }
    return true;
};

const performLogin = async () => {
    try {
        const res = await login(form);
        if (res.code === 0) {
            localStorage.setItem('token', res.data.token);
            localStorage.setItem('role_id', String(res.data.role_id));
            localStorage.setItem('name', res.data.name);
            localStorage.setItem('admin', String(res.data.role_id === 0));

            message.success('登录成功');
            
            if (res.data.requirePasswordChange) {
                router.push('/settings/password');
            } else {
                router.push('/dashboard');
            }
        } else {
            message.error(res.msg || '登录失败');
        }
    } catch (e) {
        message.error('登录失败');
    } finally {
        loading.value = false;
    }
};

const initCaptcha = async () => {
    try {
        await loadTajs();
        
        if (tacInstance.value) {
            tacInstance.value.destroyWindow();
        }

        const baseURL = axios.defaults.baseURL;
        
        const config = {
            requestCaptchaDataUrl: `${baseURL}captcha/generate`,
            validCaptchaUrl: `${baseURL}captcha/verify`,
            bindEl: "#captcha-container",
            validSuccess: (res: any, _: any, tac: any) => {
                form.captchaId = res.data.validToken;
                showCaptcha.value = false;
                tac.destroyWindow();
                performLogin();
            },
            validFail: (_: any, _captcha: any, tac: any) => tac.reloadCaptcha(),
            btnCloseFun: (_: any, tac: any) => {
                showCaptcha.value = false;
                tac.destroyWindow();
                loading.value = false;
            },
            btnRefreshFun: (_: any, tac: any) => tac.reloadCaptcha()
        };

        const isDark = osTheme.value === 'dark';
        const trackColor = isDark ? "#4a5568" : "#7db0be";

        const style = {
            bgUrl: bgImage, // Note: You need to make sure this asset exists or use a placehold
            moveTrackMaskBgColor: trackColor,
            moveTrackMaskBorderColor: trackColor
        };

        tacInstance.value = new (window as any).TAC(config, style);
        tacInstance.value.init();
        tacInstance.value.openCaptcha(); // Explicitly open or auto bind? 
        // Logic from react: bindEl is #captcha-container. 
        // React code calls new TAC, then init().
        
    } catch (e) {
        console.error(e);
        message.error('验证码初始化失败');
        loading.value = false;
        showCaptcha.value = false;
    }
};

const handleLogin = async () => {
    if (!validateForm()) return;
    
    loading.value = true;
    
    try {
        const res = await checkCaptcha();
        if (res.code !== 0) {
            message.error(res.msg || '检查验证码失败');
            loading.value = false;
            return;
        }

        if (res.data === 0) {
            await performLogin();
        } else {
            showCaptcha.value = true;
            setTimeout(() => {
                initCaptcha();
            }, 100);
        }
    } catch (e) {
        console.error(e);
        loading.value = false;
        message.error('登入异常');
    }
};

onMounted(() => {
    isWebView.value = isWebViewFunc();
});

onUnmounted(() => {
    if (tacInstance.value) {
        tacInstance.value.destroyWindow();
    }
});
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-gray-100 dark:bg-[#101014] px-4">
    <NCard class="w-full max-w-md shadow-lg" size="large">
        <div class="text-center mb-6">
            <h1 class="text-2xl font-bold">登录</h1>
            <p class="text-gray-500 mt-2 text-sm">请输入您的账号信息</p>
        </div>
        
        <div class="space-y-4">
            <NInput v-model:value="form.username" placeholder="请输入用户名" size="large" />
            <NInput v-model:value="form.password" type="password" placeholder="请输入密码" size="large" @keydown.enter="handleLogin"/>
            
            <NButton type="primary" size="large" block :loading="loading" @click="handleLogin">
                {{ loading ? '登录中...' : '登录' }}
            </NButton>
        </div>
    </NCard>

    <div class="fixed bottom-4 text-center text-xs text-gray-400">
        <p>Powered by flux-panel</p>
    </div>

    <!-- Captcha Layer -->
    <div v-if="showCaptcha" class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
        <div id="captcha-container" ref="captchaContainerRef"></div>
    </div>
  </div>
</template>
