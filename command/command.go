package command

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)

type Command struct {

}

func (t *Command) RunCommand(path string, name string, arg ...string) {
	prout, pwout := io.Pipe()
	prerr, pwerr := io.Pipe()

	cmd := exec.Command(name, arg...)
	fmt.Println("command:", name, arg)
	if path != "" {
		cmd.Dir = path
	}
	cmd.Stdout = pwout
	cmd.Stderr = pwerr

	tout := io.TeeReader(prout, os.Stdout)
	terr := io.TeeReader(prerr, os.Stderr)

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	var bout, berr bytes.Buffer

	go func() {
		if _, err := io.Copy(&bout, tout); err != nil {
			log.Fatal(err)
		}
	}()

	go func() {
		if _, err := io.Copy(&berr, terr); err != nil {
			log.Fatal(err)
		}
	}()

	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("buffered out %s\n", bout.String())
	fmt.Printf("buffered err %s\n", berr.String())
}
func (t *Command) RunWithPipe(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	fmt.Println(err)
}