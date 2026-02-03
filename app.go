package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"redock/api_gateway"
	"redock/app/controllers"
	"redock/deployment"
	"redock/dns_server"
	localproxy "redock/local_proxy"
	"redock/php_debug_adapter"
	"redock/ssh_server"
	"runtime/debug"
	"strings"

	"redock/pkg/configs"
	"redock/pkg/middleware"
	"redock/pkg/routes"
	"redock/pkg/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	_ "redock/docs" // load API Docs files (Swagger)

	_ "github.com/joho/godotenv/autoload" // load .env file automatically

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerClient "github.com/docker/docker/client"
	"github.com/gofiber/contrib/websocket"
)

// Embed a single file
//
//go:embed web/dist/index.html
var f embed.FS

// Embed a directory
//
//go:embed web/dist/*
var embedDirStatic embed.FS

// @title API
// @version 1.0
// @description This is an auto-generated API Docs.
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email your@mail.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @BasePath /api
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
func app() {

	defer recoverFunction()

	os.Setenv("REDOCK_HOST", "0.0.0.0")
	os.Setenv("REDOCK_PORT", "6001")
	os.Setenv("SERVER_READ_TIMEOUT", "60")

	// Set current version for update controller
	controllers.SetCurrentVersion(version)

	// Define Fiber config.
	config := configs.FiberConfig()

	// Define a new Fiber app with config.
	app := fiber.New(config)

	// Middlewares.
	middleware.FiberMiddleware(app) // Register Fiber's middleware for app.

	// API routes first so /api/v1/* is not 404 from static filesystem.
	routes.PublicRoutes(app)
	routes.TunnelServerRoutes(app)
	routes.TunnelClientRoutes(app)
	routes.TunnelApiRoutes(app)
	routes.PrivateRoutes(app)
	routes.LocalProxyRoutes(app)
	routes.PHPXDebugAdapterRoutes(app)
	routes.SavedCommandRoutes(app)
	routes.WebSocketRoutes(app)
	routes.DeploymentRoutes(app)
	routes.UsageRoutes(app)
	routes.APIGatewayRoutes(app)
	routes.DNSRoutes(app)
	routes.SetupVPNRoutes(app)
	routes.CloudflareRoutes(app)
	routes.EmailRoutes(app)
	routes.UpdateRoutes(app)
	// Static SPA after routes.
	app.Use("/", filesystem.New(filesystem.Config{
		Root:       http.FS(embedDirStatic),
		PathPrefix: "web/dist",
	}))

	app.Use("/ws", func(c *fiber.Ctx) error {
		// IsWebSocketUpgrade returns true if the client
		// requested upgrade to the WebSocket protocol.
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	go ssh_server.NewSSHClient().Start()

	// Shutdown callback: tüm portları kapatan stop fonksiyonları + DB flush
	utils.ShutdownCallback = func() {
		stopAllServers()
		if globalDB != nil {
			_ = globalDB.Close()
		}
	}

	// Start server (with or without graceful shutdown).
	log.Println("Server is running on http://" + os.Getenv("REDOCK_HOST") + ":" + os.Getenv("REDOCK_PORT"))
	utils.StartServerWithGracefulShutdown(app)
}

// stopAllServers tüm TCP/port dinleyicilerini kapatır (graceful shutdown için).
func stopAllServers() {
	deployment.GetDeployment().Shutdown()
	if srv := dns_server.GetServer(); srv != nil {
		_ = srv.Stop()
	}
	_ = api_gateway.GetGateway().Stop()
	localproxy.GetLocalProxyManager().StopAll()
	if a := php_debug_adapter.GetPHPDebugAdapter(); a != nil {
		a.Stop()
	}
	ssh_server.StopServer()
}

// Docker container'da komut çalıştıran fonksiyon
func runCommandInDocker(containerId, command string, cli *dockerClient.Client) (string, error) {
	// Docker exec komutunu başlat
	ctx := context.Background()
	resp, err := cli.ContainerExecCreate(ctx, containerId, container.ExecOptions{
		Cmd:          strings.Split(command, " "), // Komut ve parametreleri ayır
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	})
	if err != nil {
		return "", fmt.Errorf("Failed to create exec instance: %v", err)
	}

	// Komutu çalıştır
	execResp, err := cli.ContainerExecAttach(ctx, resp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("Failed to attach to exec instance: %v", err)
	}
	defer execResp.Close()

	// Çıktıyı oku
	var output strings.Builder
	_, err = io.Copy(&output, execResp.Reader) // Standart çıktı ve hata çıktısını okuyup birleştir
	if err != nil {
		return "", fmt.Errorf("Failed to read exec output: %v", err)
	}

	return output.String(), nil
}

func toUTF8(input string) (string, error) {
	reader := transform.NewReader(strings.NewReader(input), unicode.UTF8.NewDecoder())
	decoded, err := ioutil.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// Docker'da yeni bir shell başlatan fonksiyon
func startShellInDocker(containerId string, cli *dockerClient.Client) (types.HijackedResponse, string, error) {
	ctx := context.Background()

	// TTY ile shell başlat (nano gibi uygulamalar için gerekli)
	resp, err := cli.ContainerExecCreate(ctx, containerId, container.ExecOptions{
		Cmd:          []string{"sh"}, // Shell başlat
		Env:          []string{"TERM=xterm"},
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	})
	if err != nil {
		return types.HijackedResponse{}, "", fmt.Errorf("Failed to create exec instance: %v", err)
	}

	// Exec'e bağlan
	execResp, err := cli.ContainerExecAttach(ctx, resp.ID, container.ExecAttachOptions{Tty: true})
	if err != nil {
		return types.HijackedResponse{}, "", fmt.Errorf("Failed to attach to exec instance: %v", err)
	}

	// Terminal boyutunu ayarla (nano için uygun)
	err = cli.ContainerExecResize(ctx, resp.ID, container.ResizeOptions{
		Height: 24,
		Width:  80,
	})
	if err != nil {
		log.Println("Failed to resize terminal:", err)
	}

	return execResp, resp.ID, nil
}

func recoverFunction() {
	if r := recover(); r != nil {
		stack := string(debug.Stack())
		log.Println("[RECOVER][ERROR]", r, stack)
	}
}
