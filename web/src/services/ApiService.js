import axios from "axios";
import VueAxios from "vue-axios";

/**
 * @description service to call HTTP request via Axios
 */
class ApiService {
  /**
   * @description property to share vue instance
   */
  static vueInstance;

  /**
   * @description initialize vue axios
   */
  static init(app) {
    ApiService.vueInstance = app;
    ApiService.vueInstance.use(VueAxios, axios);
  }

  /**
   * @description set the default HTTP request headers
   */
  static setHeader() {
    ApiService.vueInstance.axios.defaults.headers.common["Accept"] =
      "application/json";
  }

  /**
   * @description send the GET HTTP request
   * @param resource: string
   * @param params: Object
   * @returns Promise
   */
  static query(resource, params) {
    return ApiService.vueInstance.axios.get(resource, params);
  }

  /**
   * @description send the GET HTTP request
   * @param resource: string
   * @param slug: string
   * @returns Promise
   */
  static get(resource, slug = "") {
    resource = window.location.protocol + '//' + window.location.hostname + (window.location.port !== '' ? ':' + window.location.port : '') + resource;
    return ApiService.vueInstance.axios.get(`${resource}${slug ? ('/' + slug) : ''}`);
  }

  /**
   * @description set the POST HTTP request
   * @param resource: string
   * @param params: Object
   * @returns Promise
   */
  static post(resource, params) {
    resource = window.location.protocol + '//' + window.location.hostname + (window.location.port !== '' ? ':' + window.location.port : '') + resource;
    return ApiService.vueInstance.axios.post(`${resource}`, params);
  }

  /**
   * @description send the UPDATE HTTP request
   * @param resource: string
   * @param slug: string
   * @param params: Object
   * @returns Promise
   */
  static update(resource, slug, params) {
    resource = window.location.protocol + '//' + window.location.hostname + (window.location.port !== '' ? ':' + window.location.port : '') + resource;
    return ApiService.vueInstance.axios.put(`${resource}/${slug}`, params);
  }

  /**
   * @description Send the PUT HTTP request
   * @param resource: string
   * @param params: Object
   * @returns Promise
   */
  static put(resource, params) {
    resource = window.location.protocol + '//' + window.location.hostname + (window.location.port !== '' ? ':' + window.location.port : '') + resource;
    return ApiService.vueInstance.axios.put(`${resource}`, params);
  }

  /**
   * @description Send the PATCH HTTP request
   * @param resource: string
   * @param params: Object
   * @returns Promise
   */
  static patch(resource, params) {
    resource = window.location.protocol + '//' + window.location.hostname + (window.location.port !== '' ? ':' + window.location.port : '') + resource;
    return ApiService.vueInstance.axios.patch(`${resource}`, params);
  }

  /**
   * @description Send the DELETE HTTP request
   * @param resource: string
   * @returns Promise
   */
  static delete(resource) {
    resource = window.location.protocol + '//' + window.location.hostname + (window.location.port !== '' ? ':' + window.location.port : '') + resource;
    return ApiService.vueInstance.axios.delete(resource);
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
    });
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
}

export default ApiService;
