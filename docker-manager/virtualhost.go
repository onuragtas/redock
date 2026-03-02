package docker_manager

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"redock/platform/database"
	"redock/platform/memory"

	"github.com/AlecAivazis/survey/v2"
)

var proxyNginx = `server {
    listen 80;
    server_name $domain;
    location / {
        proxy_pass http://$apache2host:80;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}`

var nginx = `server {
    server_name $domain;
    root /var/www/html/$folder;
    index index.html index.php;

    location / {
        index index.php;
        # Check if a file or directory index file exists, else route it to
        try_files $uri /index.php?$query_string;
    }

    # set expiration of assets to MAX for caching
    location ~* \.(ico|css|js|gif)(\?[0-9]+)?$ {
        expires max;
        log_not_found off;
    }

    location ~* \.php$ {
        fastcgi_pass $phpversion:9000;
        include fastcgi.conf;
    }

    location ~ /files {
        deny all;
        return 404;
    }
}`

var nginxProxy = `server {
    server_name $domain;
	index index.html index.php;

    location / {
        proxy_pass http://$ipAddress:$proxyPassPort;
    }

    location ~ /files {
        deny all;
        return 404;
    }
}`

var httpd = `<VirtualHost *:80>
    ProxyPassMatch ^/(.*\.php(/.*)?)$ fcgi://$phpversion:9000/var/www/html/$folder/$1
    DirectoryIndex /index.php index.php
	ServerName $domain
	DocumentRoot /var/www/html/$folder
	LogLevel info
	<Directory /var/www/html/$folder>
        DirectoryIndex index.php
        Options Indexes FollowSymLinks
        AllowOverride All
        Require all granted
     </Directory>
</VirtualHost>`

type VirtualHost struct {
	manager *DockerEnvironmentManager
}

func (t *VirtualHost) createConfig(service, domain, folder, phpVersion, typeConf, proxyPassPort string) {
	if service == "nginx" {
		t.createNginxConfig(domain, folder, phpVersion, typeConf, proxyPassPort)
	} else {
		t.createHttpdConfig(domain, folder, phpVersion)
	}
}

func (t *VirtualHost) AddVirtualHost(service, domain, folder, phpVersion, typeConf, proxyPassPort string, addHosts bool) {
	var process string
	confPath := t.GetConfigPath(service)
	if t.checkFile(confPath + "/" + domain + ".conf") {
		selectBox := &survey.Select{Message: "this conf is exists. Continue? :", Options: []string{"y", "n"}}
		err := survey.AskOne(selectBox, &process)
		if err != nil {
			log.Println(err)
		}
		if process == "n" {
			return
		}
	}

	if t.FindInHosts(domain) {
		selectBox := &survey.Select{Message: "this domain is exists. Continue? :", Options: []string{"y", "n"}}
		err := survey.AskOne(selectBox, &process)
		if err != nil {
			log.Println(err)
		}
		if process == "n" {
			return
		}
	}

	if t.manager.DevEnv {
		folder = t.manager.Username + "/" + folder
	}

	t.createConfig(service, domain, folder, phpVersion, typeConf, proxyPassPort)
	t.manager.Restart(service)
	if addHosts {
		t.addHosts(domain)
	}
}

func (t *VirtualHost) FindInHosts(domain string) bool {
	hosts, _ := ioutil.ReadFile("/etc/hosts")
	return strings.Contains(string(hosts), domain)
}

func (t *VirtualHost) GetConfigPath(service string) string {
	if service == "nginx" {
		return t.manager.NginxConfPath
	} else {
		return t.manager.HttpdConfPath
	}
}

func (t *VirtualHost) checkFile(s string) bool {
	if _, err := os.Stat(s); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func (t *VirtualHost) createNginxConfig(domain string, folder string, version string, typeConf, proxyPassPort string) {
	nginxConf := nginx
	if typeConf != "Default" {
		nginxConf = nginxProxy
	}
	nginxConf = strings.ReplaceAll(nginxConf, "$domain", domain)
	nginxConf = strings.ReplaceAll(nginxConf, "$folder", folder)
	nginxConf = strings.ReplaceAll(nginxConf, "$phpversion", version)
	nginxConf = strings.ReplaceAll(nginxConf, "$ipAddress", t.manager.GetLocalIP())
	nginxConf = strings.ReplaceAll(nginxConf, "$proxyPassPort", proxyPassPort)
	err := ioutil.WriteFile(t.GetConfigPath("nginx")+"/"+domain+".conf", []byte(nginxConf), 0644)
	if err != nil {
		log.Println(err)
	}
}

func (t *VirtualHost) createHttpdConfig(domain string, folder string, version string) {
	nginxConf := proxyNginx
	nginxConf = strings.ReplaceAll(nginxConf, "$domain", domain)
	nginxConf = strings.ReplaceAll(nginxConf, "$folder", folder)
	nginxConf = strings.ReplaceAll(nginxConf, "$phpversion", version)
	nginxConf = strings.ReplaceAll(nginxConf, "$apache2host", t.getApache2Ip())

	err := ioutil.WriteFile(t.GetConfigPath("nginx")+"/"+domain+".conf", []byte(nginxConf), 0644)
	if err != nil {
		log.Println(err)
	}

	conf := httpd
	conf = strings.ReplaceAll(conf, "$domain", domain)
	conf = strings.ReplaceAll(conf, "$folder", folder)
	conf = strings.ReplaceAll(conf, "$phpversion", version)
	err = ioutil.WriteFile(t.GetConfigPath("httpd")+"/"+domain+".conf", []byte(conf), 0644)
	if err != nil {
		log.Println(err)
	}
}

func (t *VirtualHost) addHosts(domain string) {
	var cmd *exec.Cmd
	if t.manager.DevEnv {
		cmd = exec.Command("bash", "-c", `echo "127.0.0.1 `+domain+`" >> /etc/hosts`)
	} else {
		cmd = exec.Command("sudo", "bash", "-c", `echo "127.0.0.1 `+domain+`" >> /etc/hosts`)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		fmt.Println(err)
	}
	err1 := cmd.Wait()
	if err1 != nil {
		fmt.Println(err1)
	}
}

func (t *VirtualHost) getApache2Ip() string {
	lines := strings.Split(t.manager.Env, "\n")
	for _, line := range lines {
		if strings.Contains(line, "APACHE_HOST=") {
			return strings.ReplaceAll(line, "APACHE_HOST=", "")
		}
	}
	return ""
}

func (t *VirtualHost) getXDebugIp() (string, error) {
	lines := strings.Split(t.manager.Env, "\n")
	for _, line := range lines {
		if strings.Contains(line, "XDEBUG_HOST=") {
			return strings.ReplaceAll(line, "XDEBUG_HOST=", ""), nil
		}
	}
	return "", errors.New("not found")
}

func (t *VirtualHost) VirtualHosts() []string {
	rootPath := t.GetConfigPath("nginx")

	var files []string
	filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})

	rootPath = t.GetConfigPath("httpd")
	filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})

	return files

}

// VirtualHostsWithStarred returns virtual hosts sorted with starred ones first
func (t *VirtualHost) VirtualHostsWithStarred() ([]string, []string) {
	files := t.VirtualHosts()
	starred := t.GetStarredVHosts()

	// Create a map for quick lookup
	starredMap := make(map[string]bool)
	for _, s := range starred {
		starredMap[s] = true
	}

	// Separate starred and unstarred
	var starredFiles, unstarredFiles []string
	for _, file := range files {
		if starredMap[file] {
			starredFiles = append(starredFiles, file)
		} else {
			unstarredFiles = append(unstarredFiles, file)
		}
	}

	// Return starred first, then unstarred
	result := append(starredFiles, unstarredFiles...)
	return result, starred
}

// getDataDir returns the path to the data directory
func (t *VirtualHost) getDataDir() string {
	return t.manager.GetWorkDir() + "/data"
}

// GetStarredVHosts returns the list of starred virtual hosts from memory DB.
func (t *VirtualHost) GetStarredVHosts() []string {
	db := database.GetMemoryDB()
	if db == nil {
		return nil
	}
	list := memory.FindAll[*StarredVHostEntity](db, "starred_vhosts")
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, e.Path)
	}
	return out
}

// StarVHost adds a virtual host to the starred list.
func (t *VirtualHost) StarVHost(path string) error {
	db := database.GetMemoryDB()
	if db == nil {
		return nil
	}
	existing := memory.Where[*StarredVHostEntity](db, "starred_vhosts", "Path", path)
	if len(existing) > 0 {
		return nil
	}
	return memory.Create(db, "starred_vhosts", &StarredVHostEntity{Path: path})
}

// UnstarVHost removes a virtual host from the starred list.
func (t *VirtualHost) UnstarVHost(path string) error {
	db := database.GetMemoryDB()
	if db == nil {
		return nil
	}
	list := memory.Where[*StarredVHostEntity](db, "starred_vhosts", "Path", path)
	for _, e := range list {
		if err := memory.Delete[*StarredVHostEntity](db, "starred_vhosts", e.GetID()); err != nil {
			return err
		}
	}
	return nil
}

// IsStarred checks if a virtual host is starred.
func (t *VirtualHost) IsStarred(path string) bool {
	db := database.GetMemoryDB()
	if db == nil {
		return false
	}
	list := memory.Where[*StarredVHostEntity](db, "starred_vhosts", "Path", path)
	return len(list) > 0
}

func NewVirtualHost(manager *DockerEnvironmentManager) *VirtualHost {
	return &VirtualHost{manager: manager}
}
