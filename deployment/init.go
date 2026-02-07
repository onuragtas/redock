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

	"redock/platform/database"
	"redock/platform/memory"

	"github.com/onuragtas/command"
	"gopkg.in/src-d/go-git.v4"
	config2 "gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

// Config deployment global config + proje listesi (memory'den doldurulur).
type Config struct {
	Username string                     `yaml:"username" json:"username"`
	Token    string                     `yaml:"token" json:"token"`
	Settings Settings                   `yaml:"settings" json:"settings"`
	Projects []*DeploymentProjectEntity `yaml:"projects" json:"projects"`
}

// Settings global deployment ayarları.
type Settings struct {
	CheckTime int `yaml:"check_time" json:"check_time"`
}

type Deployment struct {
	Config        Config
	Auth          *http.BasicAuth
	Cmd           command.Command
	dockerManager *docker_manager.DockerEnvironmentManager
	configMutex   sync.RWMutex
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

func (d *Deployment) db() *memory.Database {
	return database.GetMemoryDB()
}

// LoadConfigUnsafe memory DB'den config yükler (mutex yok).
func (d *Deployment) LoadConfigUnsafe() error {
	db := d.db()
	if db == nil {
		d.Config = Config{
			Username: "",
			Token:    "",
			Settings: Settings{CheckTime: 60},
			Projects: nil,
		}
		d.Auth = &http.BasicAuth{Username: d.Config.Username, Password: d.Config.Token}
		return nil
	}

	settingsList := memory.FindAll[*DeploymentSettingsEntity](db, "deployment_settings")
	if len(settingsList) == 0 {
		defaultSettings := &DeploymentSettingsEntity{
			Username:  "",
			Token:     "",
			CheckTime: 60,
		}
		if err := memory.Create(db, "deployment_settings", defaultSettings); err != nil {
			d.Config = Config{Username: "", Token: "", Settings: Settings{CheckTime: 60}, Projects: nil}
			d.Auth = &http.BasicAuth{}
			return nil
		}
		settingsList = memory.FindAll[*DeploymentSettingsEntity](db, "deployment_settings")
	}
	settings := settingsList[0]

	projectEntities := memory.FindAll[*DeploymentProjectEntity](db, "deployment_projects")

	d.Config = Config{
		Username:  settings.Username,
		Token:     settings.Token,
		Settings:  Settings{CheckTime: settings.CheckTime},
		Projects:  projectEntities,
	}
	if d.Config.Settings.CheckTime <= 0 {
		d.Config.Settings.CheckTime = 60
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
			go func(project *DeploymentProjectEntity, w *sync.WaitGroup) {
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

func (d *Deployment) Shutdown() {
	if d.runCancel != nil {
		d.runCancel()
	}
	if d.runDone != nil {
		<-d.runDone
	}
}

func (d *Deployment) Deploy(project *DeploymentProjectEntity) {
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

func (d *Deployment) RunScript(project *DeploymentProjectEntity) {
	path, err := d.CreateScript(project.Path+"script", project.Script)
	if err == nil {
		d.Cmd.RunCommand(project.Path, "chmod", "+x", path)
		d.Cmd.RunCommand(project.Path, "bash", "-c", path)
	}
}

func (d *Deployment) Checkout(project *DeploymentProjectEntity) {
	d.Cmd.RunCommand(project.Path, "git", "reset", "--hard", "HEAD")
	d.Cmd.RunCommand(project.Path, "git", "clean", "-fd")
	d.Cmd.RunCommand(project.Path, "git", "checkout", "master")
	d.Cmd.RunCommand(project.Path, "git", "branch", "-D", project.Branch)
	d.Cmd.RunCommand(project.Path, "git", "checkout", project.Branch)
	d.Cmd.RunCommand(project.Path, "git", "pull")
}

func (d *Deployment) GetList() []*DeploymentProjectEntity {
	d.configMutex.RLock()
	defer d.configMutex.RUnlock()

	d.LoadConfigUnsafe()
	if d.Config.Projects == nil {
		return nil
	}
	return d.Config.Projects
}

func (d *Deployment) GetConfig() Config {
	d.configMutex.RLock()
	defer d.configMutex.RUnlock()

	d.LoadConfigUnsafe()
	return d.Config
}

func (d *Deployment) UpdateCheckTime(checkTime int) error {
	db := d.db()
	if db == nil {
		return nil
	}
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	d.LoadConfigUnsafe()
	list := memory.FindAll[*DeploymentSettingsEntity](db, "deployment_settings")
	if len(list) == 0 {
		return nil
	}
	list[0].CheckTime = checkTime
	return memory.Update(db, "deployment_settings", list[0])
}

func (d *Deployment) AddProject(project *DeploymentProjectEntity) error {
	db := d.db()
	if db == nil {
		return nil
	}
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	return memory.Create(db, "deployment_projects", project)
}

func (d *Deployment) DeleteProject(projectPath string) error {
	db := d.db()
	if db == nil {
		return nil
	}
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	list := memory.Where[*DeploymentProjectEntity](db, "deployment_projects", "Path", projectPath)
	for _, e := range list {
		if err := memory.Delete[*DeploymentProjectEntity](db, "deployment_projects", e.GetID()); err != nil {
			return err
		}
	}
	return nil
}

func (d *Deployment) UpdateProject(project *DeploymentProjectEntity) error {
	db := d.db()
	if db == nil {
		return nil
	}
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	return memory.Update(db, "deployment_projects", project)
}

func (d *Deployment) GetProjectByPath(path string) (*DeploymentProjectEntity, error) {
	db := d.db()
	if db == nil {
		return nil, nil
	}
	d.configMutex.RLock()
	defer d.configMutex.RUnlock()

	list := memory.Where[*DeploymentProjectEntity](db, "deployment_projects", "Path", path)
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

func (d *Deployment) SetCredentials(username, token string, checkTime *int) error {
	db := d.db()
	if db == nil {
		return nil
	}
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	d.LoadConfigUnsafe()
	list := memory.FindAll[*DeploymentSettingsEntity](db, "deployment_settings")
	if len(list) == 0 {
		entity := &DeploymentSettingsEntity{
			Username:  username,
			Token:     token,
			CheckTime: 60,
		}
		if checkTime != nil {
			entity.CheckTime = *checkTime
		}
		return memory.Create(db, "deployment_settings", entity)
	}
	entity := list[0]
	entity.Username = username
	entity.Token = token
	if checkTime != nil {
		entity.CheckTime = *checkTime
	}
	return memory.Update(db, "deployment_settings", entity)
}

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

func (d *Deployment) getAuthForProject(project *DeploymentProjectEntity) *http.BasicAuth {
	username, token := d.getCredentialsForProject(project)
	return &http.BasicAuth{
		Username: username,
		Password: token,
	}
}

func (d *Deployment) getCredentialsForProject(project *DeploymentProjectEntity) (string, string) {
	username := project.Username
	token := project.Token
	if username == "" {
		username = d.Config.Username
	}
	if token == "" {
		token = d.Config.Token
	}
	return username, token
}
