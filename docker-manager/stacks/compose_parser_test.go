package stacks

import "testing"

// TestParseComposeAdvanced exercises the parser features added for broad
// docker-compose compatibility: extends, healthcheck, long-syntax ports/volumes,
// and env_file.
func TestParseComposeAdvanced(t *testing.T) {
	const compose = `
services:
  base:
    image: alpine:3.19
    environment:
      SHARED: "1"
  app:
    extends: base
    image: nginx:1.27
    env_file:
      - ./app.env
    ports:
      - target: 80
        published: 8080
        protocol: tcp
      - "53:53/udp"
    volumes:
      - type: volume
        source: appdata
        target: /data
        read_only: true
      - ./conf:/etc/nginx:ro
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
`
	ps, err := ParseCompose([]byte(compose))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var app *ServiceSpec
	for i := range ps.Services {
		if ps.Services[i].Name == "app" {
			app = &ps.Services[i]
		}
	}
	if app == nil {
		t.Fatal("app service not parsed")
	}

	// extends: base's environment is inherited.
	if app.Env["SHARED"] != "1" {
		t.Errorf("extends: SHARED not inherited, got env=%v", app.Env)
	}

	// env_file recorded.
	if len(app.EnvFile) != 1 || app.EnvFile[0] != "./app.env" {
		t.Errorf("env_file: got %v", app.EnvFile)
	}

	// ports: long-syntax tcp + short-syntax udp.
	var haveTCP, haveUDP bool
	for _, p := range app.Ports {
		if p.Host == "8080" && p.Container == "80" {
			haveTCP = true
		}
		if p.Host == "53" && p.Container == "53" && p.Protocol == "udp" {
			haveUDP = true
		}
	}
	if !haveTCP || !haveUDP {
		t.Errorf("ports: tcp=%v udp=%v got %+v", haveTCP, haveUDP, app.Ports)
	}

	// volumes: long-syntax named (read-only) + short-syntax bind.
	var haveNamed, haveBind bool
	for _, v := range app.Volumes {
		if v.Kind == VolumeNamed && v.Source == "appdata" && v.Target == "/data" && v.ReadOnly {
			haveNamed = true
		}
		if v.Kind == VolumeBind && v.Target == "/etc/nginx" && v.ReadOnly {
			haveBind = true
		}
	}
	if !haveNamed || !haveBind {
		t.Errorf("volumes: named=%v bind=%v got %+v", haveNamed, haveBind, app.Volumes)
	}

	// healthcheck parsed.
	if app.Healthcheck == nil {
		t.Fatal("healthcheck: nil")
	}
	if got := app.Healthcheck; got.Interval != "30s" || got.Retries != 3 || got.StartPeriod != "10s" {
		t.Errorf("healthcheck: got %+v", got)
	}
}
