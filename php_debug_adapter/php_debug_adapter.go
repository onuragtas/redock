package php_debug_adapter

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	docker_manager "redock/docker-manager"
	"strings"

	proxy2 "github.com/onuragtas/reverse-proxy/proxy"
)

type PHPDebugAdapter struct {
	docker_manager *docker_manager.DockerEnvironmentManager
	started        bool
	closeCh        chan bool
}

type Data struct {
	Listen   string    `json:"listen"`
	Mappings []Mapping `json:"mappings"`
}

type Mapping struct {
	Name string `json:"name"`
	Path string `json:"path"`
	URL  string `json:"url"`
}

var adapter *PHPDebugAdapter
var data *Data

func Init(docker_manager *docker_manager.DockerEnvironmentManager) {
	adapter = &PHPDebugAdapter{docker_manager: docker_manager}
	adapter.closeCh = make(chan bool)
}

func GetPHPDebugAdapter() *PHPDebugAdapter {
	return adapter
}

func (t *PHPDebugAdapter) Start() {
	data := t.getList()
	if data == nil {
		return
	}

	if t.started {
		return
	}

	t.closeCh = make(chan bool)

	listener, err := net.Listen("tcp", data.Listen)
	if err != nil {
		panic("connection error:" + err.Error())
	}
	log.Println("Proxy listening on", data.Listen, "...")

	go func(l net.Listener) {
		for {
			select {
			case <-t.closeCh:
				t.started = false
				l.Close()
				return
			}
		}
	}(listener)

	t.started = true

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("accept error:", err)
			break
		}
		proxy := proxy2.Proxy{
			Timeout:    200,
			Src:        conn,
			OnResponse: t.onResponse,
			OnRequest:  t.onRequest,
			//RequestHost: t.setDestination,
			OnCloseSource: func(conn net.Conn) {
				log.Println("Connection closed from", conn.RemoteAddr().String())
			},
			OnCloseDestination: func(conn net.Conn) {
				if conn != nil && conn.RemoteAddr() != nil && conn.RemoteAddr().String() != "" {
					log.Println("Connection closed to", conn.RemoteAddr().String())
				}
			},
			RequestTCPDestination: func(request []byte, host string, src net.Conn) net.Conn {
				data := t.getList()
				for _, mapping := range data.Mappings {
					if strings.Contains(string(request), mapping.Path) {
						cnn, _ := net.Dial("tcp", mapping.URL)
						return cnn
					}
				}
				return nil
			},
		}
		if err != nil {
			fmt.Println("Accept Error:", err)
			continue
		}
		go proxy.Handle()
	}
}

func (t *PHPDebugAdapter) onRequest(srcLocal, srcRemote, dstLocal, dstRemote string, request []byte, srcConnection net.Conn, dstConnection net.Conn) {
	log.Println(srcLocal, "->", srcRemote, "->", dstLocal, "->", dstRemote, string(request))
	if strings.Contains(string(request), "tatus=\"stopping\"") {
		srcConnection.Close()
		dstConnection.Close()
	}
}

func (t *PHPDebugAdapter) onResponse(dstRemote, dstLocal, srcRemote, srcLocal string, response []byte, srcConnection net.Conn, dstConnection net.Conn) {
	log.Println(dstRemote, "->", dstLocal, "->", srcRemote, "->", srcLocal)
}

func (t *PHPDebugAdapter) getList() *Data {
	bytes, _ := os.ReadFile(docker_manager.GetDockerManager().GetWorkDir() + "/data/settings.json")
	err := json.Unmarshal(bytes, &data)
	if err != nil {
		data = &Data{
			Listen:   "0.0.0.0:10000",
			Mappings: []Mapping{},
		}
		t.save(data)
	}
	return data
}

func (t *PHPDebugAdapter) Add(model Mapping) {
	data = t.getList()
	for _, mapping := range data.Mappings {
		if mapping.Path == model.Path {
			return
		}
	}
	data.Mappings = append(data.Mappings, model)

	t.save(data)
}

func (t *PHPDebugAdapter) Del(path string) {
	data = t.getList()
	for i, mapping := range data.Mappings {
		if mapping.Path == path {
			data.Mappings = append(data.Mappings[:i], data.Mappings[i+1:]...)
		}
	}
	t.save(data)
}

func (t *PHPDebugAdapter) Settings() *Data {
	return t.getList()
}

func (t *PHPDebugAdapter) save(d *Data) {
	marshal, err := json.Marshal(d)
	if err != nil {
		return
	}

	os.WriteFile(docker_manager.GetDockerManager().GetWorkDir()+"/data/settings.json", marshal, 777)
}

func (t *PHPDebugAdapter) Stop() {
	if t.closeCh == nil || !t.started {
		return
	}
	close(t.closeCh)
}

func (t *PHPDebugAdapter) Update(model *Data) {
	data := t.getList()
	data.Listen = model.Listen
	t.save(data)
}
