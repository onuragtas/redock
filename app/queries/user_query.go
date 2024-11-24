package queries

import (
	"encoding/json"
	"log"
	"os"
	"redock/app/models"
	docker_manager "redock/docker-manager"
)

// UserQueries struct for queries from User model.
type UserQueries struct {
}

func (u *UserQueries) getList() []models.User {
	var users []models.User
	file, err := os.ReadFile(docker_manager.GetDockerManager().GetWorkDir() + "/data/users.json")
	if err != nil {
		log.Println(err)
	}
	json.Unmarshal(file, &users)

	return users
}

// GetUserByID query for getting one User by given ID.
func (q *UserQueries) GetUserByID(id int) (models.User, error) {
	// Define User variable.
	user := models.User{}

	users := q.getList()
	for _, u := range users {
		if u.ID == id {
			return u, nil
		}
	}

	return user, nil
}

// GetUserByEmail query for getting one User by given Email.
func (q *UserQueries) GetUserByEmail(email string) (models.User, error) {
	// Define User variable.
	user := models.User{}

	users := q.getList()
	for _, u := range users {
		if u.Email == email {
			return u, nil
		}
	}

	return user, nil
}

// CreateUser query for creating a new user by given email and password hash.
func (q *UserQueries) CreateUser(u *models.User) error {
	users := q.getList()
	users = append(users, *u)
	marshall, _ := json.Marshal(users)
	os.WriteFile(docker_manager.GetDockerManager().GetWorkDir()+"/data/users.json", marshall, 0777)

	return nil
}
