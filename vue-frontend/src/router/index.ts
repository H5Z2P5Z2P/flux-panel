import { createRouter, createWebHistory } from 'vue-router'
import { isLoggedIn, isAdmin } from '@/utils/auth'
import AdminLayout from '@/layouts/AdminLayout.vue'
import GuestLayout from '@/layouts/GuestLayout.vue'

const router = createRouter({
    history: createWebHistory(import.meta.env.BASE_URL),
    routes: [
        {
            path: '/login',
            name: 'login',
            component: () => import('@/pages/login.vue'),
            meta: { title: '登录', public: true }
        },
        {
            path: '/',
            component: AdminLayout,
            redirect: '/dashboard',
            children: [
                {
                    path: 'dashboard',
                    name: 'dashboard',
                    component: () => import('@/pages/dashboard/index.vue'),
                    meta: { title: '仪表盘' }
                },
                {
                    path: 'profile',
                    name: 'profile',
                    component: () => import('@/pages/profile/index.vue'),
                    meta: { title: '个人中心' }
                },
                {
                    path: 'settings/password',
                    name: 'change-password',
                    component: () => import('@/pages/settings/password.vue'),
                    meta: { title: '修改密码' }
                },
                // Admin Only Routes
                {
                    path: 'users',
                    name: 'users',
                    component: () => import('@/pages/users/index.vue'),
                    meta: { title: '用户管理', role: 'admin' }
                },
                {
                    path: 'nodes',
                    name: 'nodes',
                    component: () => import('@/pages/nodes/index.vue'),
                    meta: { title: '节点管理', role: 'admin' }
                },
                {
                    path: 'tunnels',
                    name: 'tunnels',
                    component: () => import('@/pages/tunnels/index.vue'),
                    meta: { title: '隧道管理', role: 'admin' }
                },
                {
                    path: 'forwards',
                    name: 'forwards',
                    component: () => import('@/pages/forwards/index.vue'),
                    meta: { title: '转发配置', role: 'admin' }
                },
                {
                    path: 'limits',
                    name: 'limits',
                    component: () => import('@/pages/limits/index.vue'),
                    meta: { title: '限速管理', role: 'admin' }
                },
                {
                    path: 'config',
                    name: 'config',
                    component: () => import('@/pages/config/index.vue'),
                    meta: { title: '网站配置', role: 'admin' }
                },
                {
                    path: 'settings',
                    name: 'settings',
                    component: () => import('@/pages/settings/index.vue'),
                    meta: { title: '面板设置', role: 'admin' }
                }
            ]
        },
        {
            path: '/guest',
            component: GuestLayout,
            children: [
                {
                    path: '',
                    name: 'guest-dashboard-root',
                    redirect: 'dashboard'
                },
                {
                    path: 'dashboard',
                    name: 'guest-dashboard',
                    component: () => import('@/pages/guest/dashboard.vue'),
                    meta: { title: '访客仪表盘', public: true }
                }
            ]
        }
    ]
})

router.beforeEach((to, from, next) => {
    const isPublic = to.matched.some(record => record.meta.public);
    const logged = isLoggedIn();

    // 1. 公开页面处理
    if (isPublic) {
        if (to.name === 'login' && logged) {
            next('/'); // 已登录去首页
        } else {
            next();
        }
        return;
    }

    // 2. 未登录拦截
    if (!logged) {
        next('/login');
        return;
    }

    // 3. 权限拦截 (admin)
    if (to.meta.role === 'admin' && !isAdmin()) {
        // next('/403'); // 暂无403，重定向到 dashboard 或 profile
        next('/dashboard');
        return;
    }

    next();
});

export default router
