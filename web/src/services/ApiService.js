import axios from "axios";
import VueAxios from "vue-axios";

const REDOCK_JWT_KEY = 'redock_jwt';
const REDOCK_REFRESH_KEY = 'redock_refresh';
const TUNNEL_TOKEN_KEY = 'tunnel_token';

/**
 * @description service to call HTTP request via Axios
 */
class ApiService {
  static vueInstance;
  static defaultPort = 6001;

  static init(app) {
    ApiService.vueInstance = app;
    ApiService.vueInstance.use(VueAxios, axios);
    ApiService.setupInterceptors();
  }

  static setHeader() {
    ApiService.vueInstance.axios.defaults.headers.common["Accept"] = "application/json";
  }

  static getJWT() {
    return localStorage.getItem(REDOCK_JWT_KEY) || '';
  }

  static getRefreshToken() {
    return localStorage.getItem(REDOCK_REFRESH_KEY) || '';
  }

  static setJWT(access, refresh = '') {
    if (access) localStorage.setItem(REDOCK_JWT_KEY, access);
    // refresh undefined ise mevcut refresh'e dokunma (bazı çağrılar sadece access geçebilir)
    if (refresh !== undefined) {
      if (refresh) localStorage.setItem(REDOCK_REFRESH_KEY, refresh);
      else localStorage.removeItem(REDOCK_REFRESH_KEY);
    }
  }

  static clearJWT() {
    localStorage.removeItem(REDOCK_JWT_KEY);
    localStorage.removeItem(REDOCK_REFRESH_KEY);
  }

  static getTunnelToken() {
    return localStorage.getItem(TUNNEL_TOKEN_KEY) || '';
  }

  static setTunnelToken(token) {
    if (token) localStorage.setItem(TUNNEL_TOKEN_KEY, token);
    else localStorage.removeItem(TUNNEL_TOKEN_KEY);
  }

  static clearTunnelToken() {
    localStorage.removeItem(TUNNEL_TOKEN_KEY);
  }

  static setupInterceptors() {
    ApiService.vueInstance.axios.interceptors.request.use((config) => {
      const jwt = ApiService.getJWT();
      if (jwt) {
        config.headers.Authorization = `Bearer ${jwt}`;
      }
      const tunnelToken = ApiService.getTunnelToken();
      if (tunnelToken && (config.url || '').includes('/tunnel/')) {
        config.headers['X-Tunnel-Token'] = tunnelToken;
      }
      return config;
    });

    ApiService.vueInstance.axios.interceptors.response.use(
      (response) => response,
      async (error) => {
        if (!error.response || error.response.status !== 401) {
          return Promise.reject(error);
        }
        const isTunnelRequest = (error.config?.url || '').includes('/tunnel/');
        // Tunnel 401 = tunnel token yok; sayfada tunnel login gösterilir, Redock login'e atma.
        if (isTunnelRequest) {
          return Promise.reject(error);
        }
        // Retry sonrası yine 401 = döngüye girme, login'e at.
        if (error.config?._retriedAfterRefresh) {
          ApiService.clearJWT();
          if (!window.location.hash.includes('/login') && !window.location.pathname.includes('/login')) {
            window.location.hash = '#/login';
          }
          return Promise.reject(error);
        }
        // Renew endpoint 401 = refresh başarısız; login'e at.
        const isRenewRequest = (error.config?.url || '').includes('/token/renew');
        if (isRenewRequest) {
          ApiService.clearJWT();
          if (!window.location.hash.includes('/login') && !window.location.pathname.includes('/login')) {
            window.location.hash = '#/login';
          }
          return Promise.reject(error);
        }
        const refreshToken = ApiService.getRefreshToken();
        if (!refreshToken) {
          ApiService.clearJWT();
          if (!window.location.hash.includes('/login') && !window.location.pathname.includes('/login')) {
            window.location.hash = '#/login';
          }
          return Promise.reject(error);
        }
        const baseURL = window.location.protocol + '//' + window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : ''));
        const doRefresh = () =>
          ApiService.vueInstance.axios
            .post(baseURL + '/api/v1/token/renew', { refresh_token: refreshToken }, { _isRenewRequest: true })
            .then((res) => {
              const data = res.data;
              if (data?.tokens?.access) {
                return { access: data.tokens.access, refresh: data.tokens.refresh || '' };
              }
              throw new Error('Renew failed');
            });
        // Aynı anda gelen tüm 401'ler tek bir refresh'i bekler; refresh bitince hepsi yeni token ile retry edilir.
        if (!ApiService._refreshingPromise) {
          ApiService._refreshingPromise = doRefresh().catch((e) => {
            ApiService._refreshingPromise = null;
            ApiService.clearJWT();
            if (!window.location.hash.includes('/login') && !window.location.pathname.includes('/login')) {
              window.location.hash = '#/login';
            }
            throw e;
          });
        }
        const retryPromise = ApiService._refreshingPromise.then((tokens) => {
          ApiService.setJWT(tokens.access, tokens.refresh);
          error.config.headers = error.config.headers || {};
          error.config.headers.Authorization = 'Bearer ' + tokens.access;
          error.config._retriedAfterRefresh = true;
          return ApiService.vueInstance.axios.request(error.config);
        });
        // Bu isteğin retry'ı bittikten sonra _refreshingPromise'ı sıfırla; böylece sonraki 401'ler yeni refresh tetikler.
        return retryPromise
          .catch(() => Promise.reject(error))
          .finally(() => {
            ApiService._refreshingPromise = null;
          });
      }
    );

    ApiService.vueInstance.axios.interceptors.request.use(async (config) => {
      if (config.skipPrecheck) return config;
      if (window.location.hash.includes('/login') || window.location.pathname.includes('/login')) {
        return config;
      }
      const jwt = ApiService.getJWT();
      if (!jwt) {
        window.location.hash = '#/login';
        return config;
      }
      return config;
    });
  }

  static mergeOptions(options, skipPrecheck = false) {
    let merged = { ...options };
    if (skipPrecheck) merged.skipPrecheck = true;
    return merged;
  }

  static get(resource, { slug = "", params = {}, options = {}, skipPrecheck = false } = {}) {
    let url = window.location.protocol + '//' + window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : '')) + resource;
    if (slug) url += '/' + slug;
    return ApiService.vueInstance.axios.get(url, { ...ApiService.mergeOptions(options, skipPrecheck), params });
  }

  static post(resource, data = {}, { options = {}, skipPrecheck = false } = {}) {
    let url = window.location.protocol + '//' + window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : '')) + resource;
    return ApiService.vueInstance.axios.post(url, data, ApiService.mergeOptions(options, skipPrecheck));
  }

  static put(resource, data = {}, { options = {}, skipPrecheck = false } = {}) {
    let url = window.location.protocol + '//' + window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : '')) + resource;
    return ApiService.vueInstance.axios.put(url, data, ApiService.mergeOptions(options, skipPrecheck));
  }

  static patch(resource, data = {}, { options = {}, skipPrecheck = false } = {}) {
    let url = window.location.protocol + '//' + window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : '')) + resource;
    return ApiService.vueInstance.axios.patch(url, data, ApiService.mergeOptions(options, skipPrecheck));
  }

  static delete(resource, { options = {}, skipPrecheck = false } = {}) {
    let url = window.location.protocol + '//' + window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : '')) + resource;
    return ApiService.vueInstance.axios.delete(url, ApiService.mergeOptions(options, skipPrecheck));
  }

  static async getAuthSetup() {
    return await this.get('/api/v1/auth/setup', { skipPrecheck: true });
  }

  static async authMe() {
    return await this.get('/api/v1/auth/me', { skipPrecheck: true });
  }

  static async userInfo() {
    return await this.authMe();
  }

  static async getAllSavedCommands() {
    return await this.get('/api/v1/saved_commands');
  }

  static async getSavedCommandById(id) {
    return await this.get(`/api/v1/saved_commands/${id}`);
  }

  static async addSavedCommand(data) {
    return await this.post('/api/v1/saved_commands', data);
  }

  static async updateSavedCommand(id, data) {
    return await this.put(`/api/v1/saved_commands/${id}`, data);
  }

  static async deleteSavedCommand(id) {
    return await this.delete(`/api/v1/saved_commands/${id}`);
  }

  static async login(email, password) {
    const parameters = { email, password };
    return await this.post('/api/v1/user/sign/in', parameters, { skipPrecheck: true });
  }

  static async signUp(email, password, userRole = 'user') {
    return await this.post('/api/v1/user/sign/up', { email, password, user_role: userRole }, { skipPrecheck: true });
  }

  static logout() {
    ApiService.clearJWT();
    // Tunnel token'ı Redock logout'ta silmiyoruz; kullanıcı tekrar giriş yapsa tunnel tarafı değişmez.
  }

  static async getEnv() {
    return await this.get('/api/v1/docker/env');
  }

  static async setEnv(env) {
    return await this.post('/api/v1/docker/env', {
      env: env
    });
  }

  static async regenerateXDebugConfiguration() {
    return await this.post('/api/v1/docker/regenerate', {});
  }

  static async addXDebugConfiguration() {
    return await this.get('/api/v1/docker/add_xdebug');
  }

  static async removeXDebugConfiguration() {
    return await this.get('/api/v1/docker/remove_xdebug');
  }

  static async restartNginxHttpd() {
    return await this.get('/api/v1/docker/restart_nginx_httpd');
  }

  static async selfUpdate() {
    return await this.get('/api/v1/docker/self_update');
  }

  static async install() {
    return await this.get('/api/v1/docker/install');
  }

  static async updateDocker() {
    return await this.get('/api/v1/docker/update_docker');
  }

  static async updateDockerImages() {
    return await this.get('/api/v1/docker/update_docker_images');
  }

  static async getLocalIp() {
    return await this.get('/api/v1/docker/ip');
  }

  static async getAllServices() {
    return await this.get('/api/v1/docker/services');
  }

  static async getDockerServiceSettings() {
    return await this.get('/api/v1/docker/service_settings');
  }

  static async updateDockerServiceSettings(data) {
    return await this.post('/api/v1/docker/service_settings', data);
  }

  static async getAllVHosts() {
    return await this.get('/api/v1/docker/vhosts');
  }

  static async starVHost(path) {
    return await this.post('/api/v1/docker/star_vhost', { path: path });
  }

  static async unstarVHost(path) {
    return await this.post('/api/v1/docker/unstar_vhost', { path: path });
  }

  static async getVHostContent(path) {
    return await this.post('/api/v1/docker/get_vhost', { path: path });
  }

  static async setVHostContent(path, content) {
    return await this.post('/api/v1/docker/set_vhost', { path: path, content: content });
  }

  static async deleteVHost(path) {
    return await this.post('/api/v1/docker/delete_vhost', { path: path });
  }

  static async getVHostEnvMode(path) {
    return await this.post('/api/v1/docker/vhost_env_mode', { path: path });
  }

  static async toggleVHostEnvMode(path) {
    return await this.post('/api/v1/docker/toggle_vhost_env', { path: path });
  }

  static async getVHostTerminalInfo(path) {
    return await this.post('/api/v1/docker/vhost_terminal_info', { path: path });
  }

  static async getPhpServices() {
    return await this.get('/api/v1/docker/php_services');
  }

  static async addVHost(data) {
    return await this.post('/api/v1/docker/create_vhost', data);
  }

  static async getPersonalContainers(data) {
    return await this.get('/api/v1/docker/devenv', data);
  }

  static async addPersonalContainer(data) {
    return await this.post('/api/v1/docker/create_devenv', data);
  }

  static async editPersonalContainer(data) {
    return await this.post('/api/v1/docker/edit_devenv', data);
  }

  static async deletePersonalContainer(data) {
    return await this.post('/api/v1/docker/delete_devenv', data);
  }

  static async regeneratePersonalContainer() {
    return await this.get('/api/v1/docker/regenerate_devenv');
  }

  static async checkLogin() {
    return await this.get('/api/v1/tunnel/check_login');
  }

  static async tunnelLogin(username, password) {
    return await this.post('/api/v1/tunnel/login', {
      username: username,
      password: password
    }, { skipPrecheck: true });
  }

  static async tunnelRegister(email, username, password) {
    return await this.post('/api/v1/tunnel/register', {
      email: email,
      username: username,
      password: password,
    }, { skipPrecheck: true });
  }

  static async tunnelLogout() {
    return await this.get('/api/v1/tunnel/logout');
  }

  static async tunnelList() {
    return await this.get('/api/v1/tunnel/list');
  }

  static async tunnelDelete(data) {
    return await this.post('/api/v1/tunnel/delete', data);
  }

  static async tunnelCreate(data) {
    return await this.post('/api/v1/tunnel/add', data);
  }

  static async tunnelStart(data) {
    return await this.post('/api/v1/tunnel/start', data);
  }

  static async tunnelStop(data) {
    return await this.post('/api/v1/tunnel/stop', data);
  }

  static async tunnelRenew(data) {
    return await this.post('/api/v1/tunnel/renew', data);
  }

  static async localProxyCreate(data) {
    return await this.post('/api/v1/local_proxy/create', data);
  }

  static async localProxyList() {
    return await this.get('/api/v1/local_proxy/list');
  }

  static async localProxyStart(data) {
    return await this.post('/api/v1/local_proxy/start', data);
  }

  static async localProxyStop(data) {
    return await this.post('/api/v1/local_proxy/stop', data);
  }

  static async localProxyDelete(data) {
    return await this.post('/api/v1/local_proxy/delete', data);
  }

  static async localProxyStartAll() {
    return await this.get('/api/v1/local_proxy/start_all');
  }

  static async addService(service) {
    return await this.post('/api/v1/docker/add_service', {
      service: service
    });
  }

  static async removeService(service) {
    return await this.post('/api/v1/docker/remove_service', {
      service: service
    });
  }

  static async getXDebugAdapterSettings() {
    return await this.get('/api/v1/php_xdebug_adapter/settings');
  }

  static async addXDebugAdapterSettings(data) {
    return await this.post('/api/v1/php_xdebug_adapter/add', data);
  }

  static async removeXDebugAdapterSettings(data) {
    return await this.post('/api/v1/php_xdebug_adapter/remove', data);
  }

  static async updateXDebugAdapterSettings(data) {
    return await this.post('/api/v1/php_xdebug_adapter/update', data);
  }

  static async stopXDebugAdapter() {
    return await this.get('/api/v1/php_xdebug_adapter/stop');
  }

  static async startXDebugAdapter() {
    return await this.get('/api/v1/php_xdebug_adapter/start');
  }

  // Deployment API methods
  static async deploymentList() {
    return await this.get('/api/v1/deployment/list');
  }

  static async deploymentAdd(data) {
    return await this.post('/api/v1/deployment/add', data);
  }

  static async deploymentUpdate(data) {
    return await this.post('/api/v1/deployment/update', data);
  }

  static async deploymentDelete(data) {
    return await this.post('/api/v1/deployment/delete', data);
  }

  static async deploymentSetCredentials(data) {
    return await this.post('/api/v1/deployment/set_credentials', data);
  }

  static async deploymentGetSettings() {
    return await this.get('/api/v1/deployment/settings');
  }

  // API Gateway methods
  static async apiGatewayGetConfig() {
    return await this.get('/api/v1/api_gateway/config');
  }

  static async apiGatewayUpdateConfig(data) {
    return await this.post('/api/v1/api_gateway/config', data);
  }

  static async apiGatewayStart() {
    return await this.post('/api/v1/api_gateway/start');
  }

  static async apiGatewayStop() {
    return await this.post('/api/v1/api_gateway/stop');
  }

  static async apiGatewayStatus() {
    return await this.get('/api/v1/api_gateway/status');
  }

  static async apiGatewayStats() {
    return await this.get('/api/v1/api_gateway/stats');
  }

  static async apiGatewayHealth() {
    return await this.get('/api/v1/api_gateway/health');
  }

  static async apiGatewayBlockClient(data) {
    return await this.post('/api/v1/api_gateway/clients/block', data);
  }

  static async apiGatewayUnblockClient(data) {
    return await this.post('/api/v1/api_gateway/clients/unblock', data);
  }

  static async apiGatewayListServices() {
    return await this.get('/api/v1/api_gateway/services');
  }

  static async apiGatewayAddService(data) {
    return await this.post('/api/v1/api_gateway/services', data);
  }

  static async apiGatewayUpdateService(data) {
    return await this.put('/api/v1/api_gateway/services', data);
  }

  static async apiGatewayDeleteService(data) {
    let url = window.location.protocol + '//' + window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : '')) + '/api/v1/api_gateway/services';
    return ApiService.vueInstance.axios.delete(url, { data });
  }

  static async apiGatewayListRoutes() {
    return await this.get('/api/v1/api_gateway/routes');
  }

  static async apiGatewayAddRoute(data) {
    return await this.post('/api/v1/api_gateway/routes', data);
  }

  static async apiGatewayUpdateRoute(data) {
    return await this.put('/api/v1/api_gateway/routes', data);
  }

  static async apiGatewayDeleteRoute(data) {
    let url = window.location.protocol + '//' + window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : '')) + '/api/v1/api_gateway/routes';
    return ApiService.vueInstance.axios.delete(url, { data });
  }

  static async apiGatewayTestUpstream(data) {
    return await this.post('/api/v1/api_gateway/test_upstream', data);
  }

  static async apiGatewayHealthCheck(data) {
    return await this.post('/api/v1/api_gateway/health_check', data);
  }

  static async apiGatewayValidate(data) {
    return await this.post('/api/v1/api_gateway/validate', data);
  }

  // Certificate/Let's Encrypt methods
  static async apiGatewayCertificateInfo() {
    return await this.get('/api/v1/api_gateway/certificate');
  }

  static async apiGatewayConfigureLetsEncrypt(data) {
    return await this.post('/api/v1/api_gateway/letsencrypt', data);
  }

  static async apiGatewayRequestCertificate() {
    return await this.post('/api/v1/api_gateway/certificate/request');
  }

  static async apiGatewayRenewerStatus() {
    return await this.get('/api/v1/api_gateway/certificate/renewer');
  }

  static async apiGatewayStartRenewer() {
    return await this.post('/api/v1/api_gateway/certificate/renewer/start');
  }

  static async apiGatewayStopRenewer() {
    return await this.post('/api/v1/api_gateway/certificate/renewer/stop');
  }

  static async apiGatewayGetObservabilityStatus() {
    return await this.get('/api/v1/api_gateway/observability');
  }

  static async apiGatewayConfigureObservability(data) {
    return await this.post('/api/v1/api_gateway/observability', data);
  }

  // Update methods
  static async getCurrentVersion() {
    return await this.get('/api/updates/version');
  }

  static async getAvailableUpdates() {
    return await this.get('/api/updates/available');
  }

  static async applyUpdate(version) {
    return await this.post('/api/updates/apply', { version });
  }

}

export default ApiService;
