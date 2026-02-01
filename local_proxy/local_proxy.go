package localproxy

import (
	"fmt"
	"log"
	"net"
	docker_manager "redock/docker-manager"
	"redock/platform/database"
	"redock/platform/memory"
	"strconv"
	"strings"
	"sync"
	"time"

	proxy "github.com/onuragtas/reverse-proxy/proxy"
)

type StartItem struct {
	CloseSignal chan bool `json:"-"`
	Listener    net.Listener
}

type LocalProxy struct {
	startedList   map[int]StartItem
	dockerManager *docker_manager.DockerEnvironmentManager
}

// Item is the API DTO for local proxy entries.
type Item struct {
	Name       string `json:"name"`
	LocalPort  int    `json:"local_port"`
	Host       string `json:"host"`
	RemotePort int    `json:"remote_port"`
	Timeout    int    `json:"timeout"`
	Started    bool   `json:"started"`
}

// LocalProxyItem is the in-memory entity for local proxy entries.
type LocalProxyItem struct {
	memory.BaseEntity
	Name       string `json:"name"`
	LocalPort  int    `json:"local_port"`
	Host       string `json:"host"`
	RemotePort int    `json:"remote_port"`
	Timeout    int    `json:"timeout"`
	Started    bool   `json:"started"`
}

const localProxyTable = "local_proxy_items"

var localProxy *LocalProxy
var lock = sync.Mutex{}

func Init(dockerManager *docker_manager.DockerEnvironmentManager) {
	localProxy = &LocalProxy{dockerManager: dockerManager}
	localProxy.startedList = make(map[int]StartItem)
}

func GetLocalProxyManager() *LocalProxy {
	return localProxy
}

func (lp *LocalProxy) GetList() []Item {
	db := database.GetMemoryDB()
	entities := memory.FindAll[*LocalProxyItem](db, localProxyTable)
	list := make([]Item, 0, len(entities))
	for _, e := range entities {
		list = append(list, Item{
			Name:       e.Name,
			LocalPort:  e.LocalPort,
			Host:       e.Host,
			RemotePort: e.RemotePort,
			Timeout:    e.Timeout,
			Started:    e.Started,
		})
	}
	return list
}

func (lp *LocalProxy) StartAll() {
	list := lp.GetList()
	for _, item := range list {
		if !item.Started {
			lp.Start(item.LocalPort)
		}
	}
}

// StopAll tüm açık local proxy listener'ları kapatır (graceful shutdown için).
func (lp *LocalProxy) StopAll() {
	lock.Lock()
	ports := make([]int, 0, len(lp.startedList))
	for port := range lp.startedList {
		ports = append(ports, port)
	}
	lock.Unlock()
	for _, port := range ports {
		lp.Stop(port)
	}
}

func (lp *LocalProxy) Create(model Item) {
	db := database.GetMemoryDB()
	list := memory.FindAll[*LocalProxyItem](db, localProxyTable)
	for _, item := range list {
		if item.LocalPort == model.LocalPort {
			return
		}
	}
	entity := &LocalProxyItem{
		Name:       model.Name,
		LocalPort:  model.LocalPort,
		Host:       model.Host,
		RemotePort: model.RemotePort,
		Timeout:    model.Timeout,
		Started:    model.Started,
	}
	_ = memory.Create(db, localProxyTable, entity)
}

func (lp *LocalProxy) Delete(localPort int) {
	if _, ok := lp.startedList[localPort]; ok {
		lp.Stop(localPort)
	}
	db := database.GetMemoryDB()
	list := memory.FindAll[*LocalProxyItem](db, localProxyTable)
	for _, item := range list {
		if item.LocalPort == localPort {
			_ = memory.Delete[*LocalProxyItem](db, localProxyTable, item.GetID())
			return
		}
	}
}

func (lp *LocalProxy) Start(localPort int) {
	go lp.start(localPort)
}

func (lp *LocalProxy) Stop(localPort int) {
	if listener, ok := lp.startedList[localPort]; ok {
		listener.Listener.Close()
		delete(lp.startedList, localPort)
		listener.CloseSignal <- true
	}
}

func (lp *LocalProxy) start(localPort int) {
	if _, ok := lp.startedList[localPort]; ok {
		return
	}

	localProxy := lp.getProxy(localPort)

	localAddr := "0.0.0.0:" + strconv.Itoa(localPort)
	listener, _ := net.Listen("tcp", localAddr)
	lock.Lock()
	closeSignal := make(chan bool)
	lp.startedList[localPort] = StartItem{Listener: listener, CloseSignal: closeSignal}
	lock.Unlock()

	log.Println("Proxy listening on", localAddr, "...")
	for {
		select {
		case <-closeSignal:
			return
		default:
		}

		conn, err := listener.Accept()
		proxy := proxy.Proxy{
			Timeout: time.Duration(localProxy.Timeout),
			Src:     conn,
			OnResponse: func(dstRemote, dstLocal, srcRemote, srcLocal string, response []byte, srcConnection, dstConnection net.Conn) {
				// srcConnection.Write(response)
				// dstConnection.Write(response)
			},
			OnRequest: func(srcLocal, srcRemote, dstLocal, dstRemote string, request []byte, srcConnection, dstConnection net.Conn) {
				// srcConnection.Write(request)
				// dstConnection.Write(request)
			},
			RequestHost: func(request []byte, host string, src net.Conn) string {
				return host
			},
			RequestTCPDestination: func(request []byte, host string, src net.Conn) net.Conn {
				localAddr := strings.Split(src.LocalAddr().String(), ":")
				port, _ := strconv.Atoi(localAddr[1])
				localProxy := lp.getProxy(port)
				destination := localProxy.Host + ":" + strconv.Itoa(localProxy.RemotePort)
				tcp, _ := net.Dial("tcp", destination)
				return tcp
			},
		}
		if err != nil {
			fmt.Println("Accept Error:", err)
			continue
		}
		go proxy.Handle()
	}
}

func (lp *LocalProxy) getProxy(localPort int) *Item {
	db := database.GetMemoryDB()
	list := memory.FindAll[*LocalProxyItem](db, localProxyTable)
	for _, e := range list {
		if e.LocalPort == localPort {
			return &Item{
				Name:       e.Name,
				LocalPort:  e.LocalPort,
				Host:       e.Host,
				RemotePort: e.RemotePort,
				Timeout:    e.Timeout,
				Started:    e.Started,
			}
		}
	}
	return nil
}

func (lp *LocalProxy) GetStartedList() map[int]StartItem {
	return lp.startedList
}
