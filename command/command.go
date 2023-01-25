package command

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"
)

type Command struct {
	stdInFunction func()
	stdInDuration int
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
		log.Println(err)
	}

	fmt.Printf("buffered out %s\n", bout.String())
	fmt.Printf("buffered err %s\n", berr.String())
}
func (t *Command) RunWithPipe(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	go func() {
		if t.stdInFunction != nil {
			time.Sleep(time.Duration(t.stdInDuration) * time.Second)
			t.stdInFunction()
		}
	}()

	err := cmd.Run()
	fmt.Println(err)
}

func (t *Command) AddStdIn(duration int, f func()) {
	t.stdInFunction = f
	t.stdInDuration = duration
}
