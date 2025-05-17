package tunnel_proxy

import (
	docker_manager "redock/docker-manager"

	tunnel "github.com/onuragtas/tunnel-client"
	"github.com/onuragtas/tunnel-client/models"
)

type TunnelProxy struct {
	client        *tunnel.Client
	dockerManager *docker_manager.DockerEnvironmentManager
}

var proxy TunnelProxy

func Init(dockerManager *docker_manager.DockerEnvironmentManager) {
	proxy = TunnelProxy{client: tunnel.NewClient(), dockerManager: dockerManager}
}

func GetTunnelProxy() *TunnelProxy {
	return &proxy
}

func (t *TunnelProxy) CheckUser() bool {
	return t.client.CheckUser()
}

func (t *TunnelProxy) Login(username, password string) models.Login {
	return t.client.Login(username, password)
}

func (t *TunnelProxy) Logout() bool {
	return t.client.Logout()
}

func (t *TunnelProxy) Register(username, password, email string) models.Register {
	return t.client.Register(username, password, email)
}

func (t *TunnelProxy) ListDomain() models.Domain {
	return t.client.ListDomain()
}

func (t *TunnelProxy) DeleteDomain(id string) models.Response {
	return t.client.DeleteDomain([]string{id})
}

func (t *TunnelProxy) AddDomain(domain string) interface{} {
	return t.client.CreateDomain(domain)
}

func (t *TunnelProxy) StartTunnel(list []models.Tunnel) {
	t.client.StartTunnel(list, sshUser, sshPassword)
}

func (t *TunnelProxy) StopTunnel(domain string) {
	tunnels := t.client.GetStartedTunnels()
	for _, v := range tunnels.Data {
		if v.Domain.Domain == domain {
			v.CloseSignal <- 1
		}
	}
}

func (t *TunnelProxy) GetStartedList() tunnel.StartedTunnels {
	return t.client.GetStartedTunnels()
}

func (t *TunnelProxy) UserInfo() models.UserInfo {
	return t.client.UserInfo()
}
