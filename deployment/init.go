package deployment

import (
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
}

var deployment *Deployment

func Init(dockerManager *docker_manager.DockerEnvironmentManager) {
	deployment = &Deployment{
		dockerManager: dockerManager,
		Cmd:           command.Command{},
	}
}

func GetDeployment() *Deployment {
	return deployment
}

func (d *Deployment) LoadConfig() error {
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

func (d *Deployment) Run() {
	var wg sync.WaitGroup
	for {
		if err := d.LoadConfig(); err != nil {
			log.Println("Config load error:", err)
			return
		}

		wg.Add(len(d.Config.Projects))
		for _, project := range d.Config.Projects {
			go func(project Project, w *sync.WaitGroup) {
				defer w.Done()
				d.Deploy(project)
			}(project, &wg)
		}

		wg.Wait()
		time.Sleep(time.Duration(d.Config.Settings.CheckTime) * time.Second)
	}
}

func (d *Deployment) Deploy(project Project) {
	_, err := git.PlainClone(project.Path, false, &git.CloneOptions{
		ReferenceName: plumbing.NewBranchReferenceName(project.Branch),
		Auth:          d.Auth,
		URL:           project.Url,
	})

	repo, err := git.PlainOpen(project.Path)
	if err != nil {
		log.Println(project, err)
		return
	}

	d.Cmd.RunCommand(project.Path, "git", "remote", "set-url", "origin", "https://"+d.Config.Username+":"+d.Config.Token+"@"+strings.ReplaceAll(project.Url, "https://", ""))

	spec := "refs/heads/" + project.Branch + ":refs/remotes/origin/" + project.Branch

	err = repo.Fetch(&git.FetchOptions{
		RefSpecs: []config2.RefSpec{config2.RefSpec(spec)},
		Auth:     d.Auth,
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
	d.LoadConfig()
	if d.Config.Projects == nil {
		return []Project{}
	}
	return d.Config.Projects
}

func (d *Deployment) GetConfig() Config {
	d.LoadConfig()
	return d.Config
}

// update check time
func (d *Deployment) UpdateCheckTime(checkTime int) error {
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
	d.LoadConfig()
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
	d.LoadConfig()
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
	d.LoadConfig()
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
	d.LoadConfig()
	for _, project := range d.Config.Projects {
		if project.Path == path {
			return &project, nil
		}
	}
	return nil, nil // or return an error if preferred
}

// SetCredentials sets the username, token, and checkTime for deployment config and saves it.
func (d *Deployment) SetCredentials(username, token string, checkTime *int) error {
	d.LoadConfig()
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
