export const siteConfig = {
    name: "Flux Panel",
    description: "Modern network traffic management panel",
    version: "2.0.0",
    app_version: "2.0.0"
};

export const getCachedConfigs = () => {
    return localStorage.getItem('site_config') ? JSON.parse(localStorage.getItem('site_config') as string) : {};
};

export const clearConfigCache = () => {
    localStorage.removeItem('site_config');
};

export const updateSiteConfig = async () => {
    // mock implementation
};
