package database

import (
	"os"
	"redock/app/queries"
	docker_manager "redock/docker-manager"
)

// Queries struct for collect all app queries.
type Queries struct {
	*queries.UserQueries // load queries from User model
}

// OpenDBConnection func for opening database connection.
func OpenDBConnection() (*Queries, error) {
	_, err := os.Stat(docker_manager.GetDockerManager().GetWorkDir() + "/data")
	if err != nil && os.IsNotExist(err) {
		os.Mkdir(docker_manager.GetDockerManager().GetWorkDir()+"/data", 0777)
	}

	return &Queries{
		// Set queries from models:
		UserQueries: &queries.UserQueries{}, // from User model
	}, nil
}
