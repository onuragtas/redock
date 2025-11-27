import axios from "axios";
import VueAxios from "vue-axios";

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

  static setupInterceptors() {
    ApiService.vueInstance.axios.interceptors.request.use(async (config) => {
      if (config.skipPrecheck) return config;
      
      // Login sayfasındaysa authentication kontrolü yapma
      if (window.location.hash.includes('/login') || window.location.pathname.includes('/login')) {
        return config;
      }
      
      try {
        const resource = window.location.protocol + '//' + window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : ''));
        const response = await ApiService.vueInstance.axios.get(resource + '/api/v1/tunnel/user_info', { skipPrecheck: true });
        if (response.data.data.id > 0) {
          return config;
        } else {
          window.location.hash = '#/login';
        }
      } catch (e) {
        window.location.hash = '#/login';
      }
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

  static async userInfo() {
    return await this.get('/api/v1/tunnel/user_info', { skipPrecheck: true });
  }

  static async getAllSavedCommands() {
    return await this.get('/api/v1/saved_commands/list');
  }

  static async addSavedCommand(data) {
    return await this.post('/api/v1/saved_commands/add', data);
  }

  static async deleteSavedCommand(data) {
    return await this.post('/api/v1/saved_commands/remove', data);
  }

  static async login(login, pass) {
    const parameters = {
      email: login,
      password: pass
    };
    return await this.post('/api/v1/user/sign/in', parameters);
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

}

export default ApiService;
