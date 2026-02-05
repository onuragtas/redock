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

// tokenPtr returns *string for tunnel-client; boş token için nil döner.
func tokenPtr(token string) *string {
	if token == "" {
		return nil
	}
	return &token
}

func (t *TunnelProxy) CheckUser(token string) bool {
	return t.client.CheckUser(tokenPtr(token))
}

func (t *TunnelProxy) Login(email, password string) models.Login {
	return t.client.Login(email, password)
}

func (t *TunnelProxy) Logout() bool {
	return t.client.Logout()
}

func (t *TunnelProxy) Register(email, password string) models.Register {
	return t.client.Register(email, password, email)
}

func (t *TunnelProxy) ListDomain(token string) models.Domain {
	return t.client.ListDomain(tokenPtr(token))
}

func (t *TunnelProxy) DeleteDomain(id string, token string) models.Response {
	return t.client.DeleteDomain([]string{id}, tokenPtr(token))
}

func (t *TunnelProxy) AddDomain(domain string, token string) interface{} {
	return t.client.CreateDomain(domain, tokenPtr(token))
}

func (t *TunnelProxy) StartTunnel(list []models.Tunnel) {
	t.client.StartTunnel(list, sshUser, sshPassword)
}

func (t *TunnelProxy) StopTunnel(domain string) {
	tunnels := t.client.GetStartedTunnels()
	for _, v := range tunnels.Data {
		if v.Domain.Domain == domain {
			t.client.CloseTunnel([]string{v.Domain.Domain})
		}
	}
}

func (t *TunnelProxy) GetStartedList() tunnel.StartedTunnels {
	return t.client.GetStartedTunnels()
}

func (t *TunnelProxy) UserInfo(token string) models.UserInfo {
	return t.client.UserInfo(tokenPtr(token))
}

func (t *TunnelProxy) RenewDomain(domain string, token string) {
	t.client.RenewDomain(domain, tokenPtr(token))
}
