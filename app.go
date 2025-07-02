package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"redock/app/models"
	"redock/platform/database"
	"redock/ssh_server"
	"runtime/debug"
	"strings"
	"time"

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
	// Define Fiber config.
	config := configs.FiberConfig()

	// Define a new Fiber app with config.
	app := fiber.New(config)

	createAdmin()

	// Middlewares.
	middleware.FiberMiddleware(app) // Register Fiber's middleware for app.

	// Access file "image.png" under `assets/` directory via URL: `http://<server>/assets/image.png`.
	// Without `PathPrefix`, you have to access it via URL:
	// `http://<server>/assets/assets/image.png`.
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

	// Routes.
	routes.PublicRoutes(app)           // Register a public routes for app.
	routes.PrivateRoutes(app)          // Register a private routes for app.
	routes.TunnelRoutes(app)           // Register a tunnel routes for app.
	routes.LocalProxyRoutes(app)       // Register a local proxy routes for app.
	routes.PHPXDebugAdapterRoutes(app) // Register a local proxy routes for app.
	routes.SavedCommandRoutes(app)     // Register a local proxy routes for app.
	routes.WebSocketRoutes(app)        // Register a websocket routes for app.
	routes.DeploymentRoutes(app)       // Register a deployment routes for app.

	// Start server (with or without graceful shutdown).
	log.Println("Server is running on http://" + os.Getenv("REDOCK_HOST") + ":" + os.Getenv("REDOCK_PORT"))
	utils.StartServerWithGracefulShutdown(app)
}

func createAdmin() {
	db, err := database.OpenDBConnection()
	if err != nil {
		log.Fatalln(errors.New("database connection error"))
	}

	findUser, err := db.UserQueries.GetUserByEmail("admin")

	if findUser.Email != "" {
		return
	}

	// Create a new user struct.
	user := &models.User{}

	// Set initialized default data for user:
	user.CreatedAt = time.Now()
	user.Email = "admin"
	user.PasswordHash = utils.GeneratePassword("admin")
	user.UserStatus = 1 // 0 == blocked, 1 == active
	user.UserRole = "admin"

	// Create a new user with validated data.
	if err := db.CreateUser(user); err != nil {
		log.Fatalln(errors.New("database error"))
	}

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
