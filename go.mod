module github.com/onuragtas/docker-env

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.3.2
	github.com/Microsoft/go-winio v0.5.1 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/kevinburke/ssh_config v1.1.0 // indirect
	github.com/kr/binarydist v0.1.0
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/sanbornm/go-selfupdate v0.0.0-20210106163404-c9b625feac49
	github.com/sergi/go-diff v1.2.0 // indirect
	github.com/xanzy/ssh-agent v0.3.1 // indirect
	golang.org/x/crypto v0.0.0-20220126173729-e04a8579fee6 // indirect
	golang.org/x/net v0.0.0-20220121210141-e204ce36a2ba // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	gopkg.in/inconshreveable/go-update.v0 v0.0.0-20150814200126-d8b0b1d421aa
	gopkg.in/src-d/go-git.v4 v4.13.1
	gopkg.in/yaml.v2 v2.4.0
)

//replace github.com/AlecAivazis/survey/v2 => github.com/onuragtas/survey/v2 v2.3.2
