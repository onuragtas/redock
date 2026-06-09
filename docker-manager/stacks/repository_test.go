package stacks

import "testing"

const sampleCompose = `
version: '3'
services:
  nginx:
    build: ./nginx
    container_name: nginx
    restart: always
    ports:
      - ${NGINX_PORT}:80
    volumes:
      - ${NGINX_CONF_PATH}:/etc/nginx/conf.d/
      - ./etc/nginx.conf:/etc/nginx/nginx.conf
    networks:
      net:
        ipv4_address: ${NGINX_HOST}
  redis:
    image: redis:alpine
    ports:
      - ${REDIS_PORT}:6379
    volumes:
      - ./etc/redis.conf:/usr/local/etc/redis/redis.conf
    ulimits:
      nofile:
        soft: 100000
        hard: 100000
    networks:
      net:
        ipv4_address: ${REDIS_HOST}
  php74_xdebug:
    image: onuragtas/php7.4-fpm-xdebug
    tty: true
    links:
      - global
    expose:
      - 9000
    volumes:
      - ./php74/php.ini:/usr/local/etc/php/php.ini
networks:
  net:
    ipam:
      config:
        - subnet: 172.28.0.0/16
`

func TestParseCompose(t *testing.T) {
	ps, err := ParseCompose([]byte(sampleCompose))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.Services) != 3 {
		t.Fatalf("want 3 services, got %d", len(ps.Services))
	}
	byName := map[string]ServiceSpec{}
	for _, s := range ps.Services {
		byName[s.Name] = s
	}
	if byName["nginx"].Build == nil || byName["nginx"].Build.Context != "nginx" {
		t.Errorf("nginx build context wrong: %+v", byName["nginx"].Build)
	}
	if byName["redis"].Image != "redis:alpine" {
		t.Errorf("redis image wrong: %q", byName["redis"].Image)
	}
	if len(byName["redis"].Ulimits) != 1 || byName["redis"].Ulimits[0].Soft != 100000 {
		t.Errorf("redis ulimit wrong: %+v", byName["redis"].Ulimits)
	}
	if !contains(byName["php74_xdebug"].DependsOn, "global") {
		t.Errorf("php74_xdebug should depend on global: %+v", byName["php74_xdebug"].DependsOn)
	}
	// BindFiles should include the repo-relative config files but NOT env-paths.
	wantBinds := map[string]bool{"etc/nginx.conf": false, "etc/redis.conf": false, "php74/php.ini": false}
	for _, f := range ps.BindFiles {
		if _, ok := wantBinds[f]; ok {
			wantBinds[f] = true
		}
	}
	for f, found := range wantBinds {
		if !found {
			t.Errorf("expected bind file %q in %v", f, ps.BindFiles)
		}
	}
	if !contains(ps.BuildContexts, "nginx") {
		t.Errorf("expected nginx build context, got %v", ps.BuildContexts)
	}
}

func TestRegistryCustomService(t *testing.T) {
	reg := NewRegistry(t.TempDir())
	if err := reg.AddCustomService(ServiceSpec{Name: "mytool", Image: "nginx:alpine"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddCustomService(ServiceSpec{Name: "bad", Build: &BuildSpec{Context: "x"}}); err == nil {
		t.Error("custom service with Build should be rejected")
	}
}

func contains(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}
