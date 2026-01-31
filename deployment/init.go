package deployment

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"os"
	docker_manager "redock/docker-manager"
	"strings"
	"sync"
	"time"

	"github.com/onuragtas/command"
	"gopkg.in/src-d/go-git.v4"
	config2 "gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Username string    `yaml:"username" json:"username"`
	Token    string    `yaml:"token" json:"token"`
	Settings Settings  `yaml:"settings" json:"settings"`
	Projects []Project `yaml:"projects" json:"projects"`
}
type Project struct {
	Url          string    `yaml:"url" json:"url"`
	Path         string    `yaml:"path" json:"path"`
	Branch       string    `yaml:"branch" json:"branch"`
	Check        string    `yaml:"check" json:"check"`
	Script       string    `yaml:"script" json:"script"`
	Username     string    `yaml:"username,omitempty" json:"username,omitempty"`
	Token        string    `yaml:"token,omitempty" json:"token,omitempty"`
	LastDeployed time.Time `yaml:"last_deployed" json:"last_deployed"`
	LastChecked  time.Time `yaml:"last_checked" json:"last_checked"`
	Enabled      bool      `yaml:"enabled" json:"enabled"`
}

type Settings struct {
	CheckTime int `yaml:"check_time" json:"check_time"`
}

type Deployment struct {
	Config        Config
	Auth          *http.BasicAuth
	Cmd           command.Command
	dockerManager *docker_manager.DockerEnvironmentManager
	configMutex   sync.RWMutex // dosya okuma/yazma için mutex
	runCtx        context.Context
	runCancel     context.CancelFunc
	runDone       chan struct{}
}

var deployment *Deployment

func Init(dockerManager *docker_manager.DockerEnvironmentManager) {
	ctx, cancel := context.WithCancel(context.Background())
	deployment = &Deployment{
		dockerManager: dockerManager,
		Cmd:           command.Command{},
		runCtx:        ctx,
		runCancel:     cancel,
		runDone:       make(chan struct{}),
	}
}

func GetDeployment() *Deployment {
	return deployment
}

// LoadConfigUnsafe mutex kullanmadan config yükler (internal use only)
func (d *Deployment) LoadConfigUnsafe() error {
	byteArray, err1 := os.ReadFile(d.dockerManager.GetWorkDir() + "/data/deployment.json")
	err := yaml.Unmarshal(byteArray, &d.Config)

	if err != nil || err1 != nil {
		// create default config if file does not exist
		if os.IsNotExist(err1) {
			d.Config = Config{
				Username: "",
				Token:    "",
				Settings: Settings{
					CheckTime: 60, // default check time in seconds
				},
				Projects: []Project{},
			}
		}
		err = yaml.Unmarshal(byteArray, &d.Config)
		data, _ := yaml.Marshal(d.Config)
		err = os.WriteFile(d.dockerManager.GetWorkDir()+"/data/deployment.json", data, 0777)
	}

	d.Auth = &http.BasicAuth{
		Username: d.Config.Username,
		Password: d.Config.Token,
	}
	return nil
}

func (d *Deployment) LoadConfig() error {
	d.configMutex.Lock()
	defer d.configMutex.Unlock()
	return d.LoadConfigUnsafe()
}

func (d *Deployment) Run() {
	defer close(d.runDone)
	for {
		select {
		case <-d.runCtx.Done():
			return
		default:
		}
		if err := d.LoadConfig(); err != nil {
			log.Println("Config load error:", err)
			return
		}

		var wg sync.WaitGroup
		wg.Add(len(d.Config.Projects))
		for _, project := range d.Config.Projects {
			go func(project Project, w *sync.WaitGroup) {
				defer w.Done()
				d.Deploy(project)
			}(project, &wg)
		}

		wg.Wait()
		select {
		case <-d.runCtx.Done():
			return
		case <-time.After(time.Duration(d.Config.Settings.CheckTime) * time.Second):
		}
	}
}

// Shutdown Run() döngüsünü durdurur ve bitmesini bekler (graceful shutdown için).
func (d *Deployment) Shutdown() {
	if d.runCancel != nil {
		d.runCancel()
	}
	if d.runDone != nil {
		<-d.runDone
	}
}

func (d *Deployment) Deploy(project Project) {
	// Proje bazlı auth kullan, yoksa global auth
	auth := d.getAuthForProject(project)
	username, token := d.getCredentialsForProject(project)

	_, err := git.PlainClone(project.Path, false, &git.CloneOptions{
		ReferenceName: plumbing.NewBranchReferenceName(project.Branch),
		Auth:          auth,
		URL:           project.Url,
	})

	repo, err := git.PlainOpen(project.Path)
	if err != nil {
		log.Println(project, err)
		return
	}

	d.Cmd.RunCommand(project.Path, "git", "remote", "set-url", "origin", "https://"+username+":"+token+"@"+strings.ReplaceAll(project.Url, "https://", ""))

	spec := "refs/heads/" + project.Branch + ":refs/remotes/origin/" + project.Branch

	err = repo.Fetch(&git.FetchOptions{
		RefSpecs: []config2.RefSpec{config2.RefSpec(spec)},
		Auth:     auth,
	})
	// if err != nil && err != git.NoErrAlreadyUpToDate {
	// 	log.Println(project, err)
	// 	return
	// }

	localBranch, _ := repo.Branches()
	remoteRefs, _ := repo.Storer.IterReferences()

	remoteBranchRef, err := remoteRefs.Next()
	for err == nil {
		if remoteBranchRef.Name().String() == "refs/remotes/origin/"+project.Branch {
			break
		}
		remoteBranchRef, err = remoteRefs.Next()
	}
	if err != nil {
		log.Println(project, err)
		return
	}

	localBranchRef, err := localBranch.Next()
	for err == nil {
		if localBranchRef.Name().String() == "refs/heads/"+project.Branch {
			break
		}
		localBranchRef, err = localBranch.Next()
	}
	if err != nil {
		d.Checkout(project)
		log.Println(project, err)
		return
	}

	remoteCommits, _ := repo.Log(&git.LogOptions{From: remoteBranchRef.Hash()})
	localCommits, _ := repo.Log(&git.LogOptions{From: localBranchRef.Hash()})

	remoteUpdated := false
	for {
		if !(localCommits == nil || remoteCommits == nil) {
			remoteCommit, remoteErr := remoteCommits.Next()
			localCommit, localErr := localCommits.Next()

			if remoteErr == io.EOF || localErr == io.EOF {
				break
			}

			if remoteCommit.Hash != localCommit.Hash {
				remoteUpdated = true
				break
			}
		} else {
			remoteUpdated = true
			break
		}
	}

	if remoteUpdated {
		d.Checkout(project)
		d.RunScript(project)
		project.LastDeployed = time.Now()
	} else if project.Check != "" {
		path, err := d.CreateScript(project.Path+"check", project.Check)
		if err == nil {
			out, _ := d.Cmd.Run(path)
			if strings.Contains(string(out), "start_deployment") {
				d.Checkout(project)
				d.RunScript(project)
				project.LastDeployed = time.Now()
			}
		}
	}
	project.LastChecked = time.Now()
	d.UpdateProject(project)
}

func (d *Deployment) RunScript(project Project) {
	path, err := d.CreateScript(project.Path+"script", project.Script)
	if err == nil {
		d.Cmd.RunCommand(project.Path, "chmod", "+x", path)
		d.Cmd.RunCommand(project.Path, "bash", "-c", path)
	}
}

func (d *Deployment) Checkout(project Project) {
	d.Cmd.RunCommand(project.Path, "git", "reset", "--hard", "HEAD")
	d.Cmd.RunCommand(project.Path, "git", "clean", "-fd")
	d.Cmd.RunCommand(project.Path, "git", "checkout", "master")
	d.Cmd.RunCommand(project.Path, "git", "branch", "-D", project.Branch)
	d.Cmd.RunCommand(project.Path, "git", "checkout", project.Branch)
	d.Cmd.RunCommand(project.Path, "git", "pull")
}

func (d *Deployment) GetList() []Project {
	d.configMutex.RLock()
	defer d.configMutex.RUnlock()

	d.LoadConfigUnsafe()
	if d.Config.Projects == nil {
		return []Project{}
	}
	return d.Config.Projects
}

func (d *Deployment) GetConfig() Config {
	d.configMutex.RLock()
	defer d.configMutex.RUnlock()

	d.LoadConfigUnsafe()
	return d.Config
}

// update check time
func (d *Deployment) UpdateCheckTime(checkTime int) error {
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	d.LoadConfigUnsafe()
	d.Config.Settings.CheckTime = checkTime
	data, err := yaml.Marshal(d.Config)
	if err != nil {
		return err
	}
	err = os.WriteFile(d.dockerManager.GetWorkDir()+"/data/deployment.json", data, 0644)
	if err != nil {
		return err
	}
	return nil
}

// add project
func (d *Deployment) AddProject(project Project) error {
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	d.LoadConfigUnsafe()
	d.Config.Projects = append(d.Config.Projects, project)
	data, err := yaml.Marshal(d.Config)
	if err != nil {
		return err
	}
	err = os.WriteFile(d.dockerManager.GetWorkDir()+"/data/deployment.json", data, 0644)
	if err != nil {
		return err
	}
	return nil
}

// delete project
func (d *Deployment) DeleteProject(projectPath string) error {
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	d.LoadConfigUnsafe()
	for i, project := range d.Config.Projects {
		if project.Path == projectPath {
			d.Config.Projects = append(d.Config.Projects[:i], d.Config.Projects[i+1:]...)
			data, err := yaml.Marshal(d.Config)
			if err != nil {
				return err
			}
			err = os.WriteFile(d.dockerManager.GetWorkDir()+"/data/deployment.json", data, 0644)
			if err != nil {
				return err
			}
			return nil
		}
	}
	return nil
}

// update project
func (d *Deployment) UpdateProject(project Project) error {
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	d.LoadConfigUnsafe()
	for i, p := range d.Config.Projects {
		if p.Path == project.Path {
			d.Config.Projects[i] = project
			data, err := yaml.Marshal(d.Config)
			if err != nil {
				return err
			}
			err = os.WriteFile(d.dockerManager.GetWorkDir()+"/data/deployment.json", data, 0644)
			if err != nil {
				return err
			}
			return nil
		}
	}
	return nil
}

// GetProjectByPath returns a project by its path.
func (d *Deployment) GetProjectByPath(path string) (*Project, error) {
	d.configMutex.RLock()
	defer d.configMutex.RUnlock()

	d.LoadConfigUnsafe()
	for _, project := range d.Config.Projects {
		if project.Path == path {
			return &project, nil
		}
	}
	return nil, nil // or return an error if preferred
}

// SetCredentials sets the username, token, and checkTime for deployment config and saves it.
func (d *Deployment) SetCredentials(username, token string, checkTime *int) error {
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	d.LoadConfigUnsafe()
	d.Config.Username = username
	d.Config.Token = token
	if checkTime != nil {
		d.Config.Settings.CheckTime = *checkTime
	}
	data, err := yaml.Marshal(d.Config)
	if err != nil {
		return err
	}
	err = os.WriteFile(d.dockerManager.GetWorkDir()+"/data/deployment.json", data, 0644)
	if err != nil {
		return err
	}
	return nil
}

// create script random name in tmp directory
func (d *Deployment) CreateScript(projectPath, scriptContent string) (string, error) {
	hash := md5.Sum([]byte(projectPath))
	scriptPath := fmt.Sprintf("%s/deploy_script_%x.sh", os.TempDir(), hash)

	file, err := os.Create(scriptPath)
	if err != nil {
		return "", err
	}

	if _, err := file.WriteString(scriptContent); err != nil {
		file.Close()
		os.Remove(scriptPath)
		return "", err
	}

	if err := file.Close(); err != nil {
		os.Remove(scriptPath)
		return "", err
	}

	return scriptPath, nil
}

// getAuthForProject returns auth for project, falls back to global if project auth is empty
func (d *Deployment) getAuthForProject(project Project) *http.BasicAuth {
	username, token := d.getCredentialsForProject(project)
	return &http.BasicAuth{
		Username: username,
		Password: token,
	}
}

// getCredentialsForProject returns username and token for project, falls back to global if empty
func (d *Deployment) getCredentialsForProject(project Project) (string, string) {
	username := project.Username
	token := project.Token

	// Eğer proje seviyesinde username/token yoksa global olanı kullan
	if username == "" {
		username = d.Config.Username
	}
	if token == "" {
		token = d.Config.Token
	}

	return username, token
}
