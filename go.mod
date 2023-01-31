module github.com/onuragtas/docker-env

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.3.6
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/kr/binarydist v0.1.0
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/sanbornm/go-selfupdate v0.0.0-20210106163404-c9b625feac49
	golang.org/x/term v0.4.0 // indirect
	golang.org/x/text v0.6.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/inconshreveable/go-update.v0 v0.0.0-20150814200126-d8b0b1d421aa
	gopkg.in/src-d/go-git.v4 v4.13.1
	gopkg.in/yaml.v2 v2.4.0
)

//replace github.com/AlecAivazis/survey/v2 => github.com/onuragtas/survey/v2 v2.3.2
