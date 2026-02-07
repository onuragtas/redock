package php_debug_adapter

import (
	"fmt"
	"log"
	"net"
	"strings"

	docker_manager "redock/docker-manager"
	"redock/platform/database"
	"redock/platform/memory"

	proxy2 "github.com/onuragtas/reverse-proxy/proxy"
)

type PHPDebugAdapter struct {
	docker_manager *docker_manager.DockerEnvironmentManager
	started        bool
	closeCh        chan bool
}

// Data API ve proxy tarafında kullanılan DTO (memory'den doldurulur).
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

func (t *PHPDebugAdapter) db() *memory.Database {
	return database.GetMemoryDB()
}

func (t *PHPDebugAdapter) getList() *Data {
	db := t.db()
	if db == nil {
		data = &Data{Listen: "0.0.0.0:10000", Mappings: []Mapping{}}
		return data
	}
	settingsList := memory.FindAll[*PhpXDebugSettingsEntity](db, "php_xdebug_settings")
	listen := "0.0.0.0:10000"
	if len(settingsList) > 0 {
		listen = settingsList[0].Listen
		if listen == "" {
			listen = "0.0.0.0:10000"
		}
	}
	mappingEntities := memory.FindAll[*PhpXDebugMappingEntity](db, "php_xdebug_mappings")
	mappings := make([]Mapping, 0, len(mappingEntities))
	for _, e := range mappingEntities {
		mappings = append(mappings, Mapping{Name: e.Name, Path: e.Path, URL: e.URL})
	}
	data = &Data{Listen: listen, Mappings: mappings}
	return data
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

func (t *PHPDebugAdapter) Add(model Mapping) {
	db := t.db()
	if db == nil {
		return
	}
	list := memory.Where[*PhpXDebugMappingEntity](db, "php_xdebug_mappings", "Path", model.Path)
	if len(list) > 0 {
		return
	}
	entity := &PhpXDebugMappingEntity{Name: model.Name, Path: model.Path, URL: model.URL}
	if err := memory.Create(db, "php_xdebug_mappings", entity); err != nil {
		return
	}
	t.getList()
}

func (t *PHPDebugAdapter) Del(path string) {
	db := t.db()
	if db == nil {
		return
	}
	list := memory.Where[*PhpXDebugMappingEntity](db, "php_xdebug_mappings", "Path", path)
	for _, e := range list {
		_ = memory.Delete[*PhpXDebugMappingEntity](db, "php_xdebug_mappings", e.GetID())
	}
	t.getList()
}

func (t *PHPDebugAdapter) Settings() *Data {
	return t.getList()
}

func (t *PHPDebugAdapter) Stop() {
	if t.closeCh == nil || !t.started {
		return
	}
	close(t.closeCh)
}

func (t *PHPDebugAdapter) Update(model *Data) {
	db := t.db()
	if db == nil {
		return
	}
	listen := model.Listen
	if listen == "" {
		listen = "0.0.0.0:10000"
	}
	settingsList := memory.FindAll[*PhpXDebugSettingsEntity](db, "php_xdebug_settings")
	if len(settingsList) == 0 {
		entity := &PhpXDebugSettingsEntity{Listen: listen}
		_ = memory.Create(db, "php_xdebug_settings", entity)
	} else {
		settingsList[0].Listen = listen
		_ = memory.Update(db, "php_xdebug_settings", settingsList[0])
	}
	t.getList()
}
