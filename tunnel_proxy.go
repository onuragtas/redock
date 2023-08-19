package main

import (
	"github.com/AlecAivazis/survey/v2"
	tunnel "github.com/onuragtas/tunnel-client"
	"github.com/onuragtas/tunnel-client/models"
	"log"
	"strconv"
)

var client = tunnel.NewClient()

func tunnelProxy() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Println("Recovered in f", r)
			}
		}()
	}()
	var process string

	if !client.CheckUser() {
		var processes = []string{"Register", "Login"}
		selectBox := &survey.Select{Message: "Pick your process", Options: processes, PageSize: 20}
		err := survey.AskOne(selectBox, &process)
		if err != nil {
			log.Println(err)
		}

		if process == "Login" {
			var username string
			var password string
			survey.AskOne(&survey.Input{Message: "Username:"}, &username)
			survey.AskOne(&survey.Input{Message: "Password:"}, &password)
			client.Login(username, password)
		} else if process == "Register" {
			var username string
			var email string
			var password string
			survey.AskOne(&survey.Input{Message: "Username:"}, &username)
			survey.AskOne(&survey.Input{Message: "Email:"}, &email)
			survey.AskOne(&survey.Input{Message: "Password:"}, &password)
			client.Register(username, password, email)
		}
	} else {
		var processes = []string{"List Domain", "Create New Domain", "Delete Domain", "Renew Domain", "Start Tunnel", "Close Tunnel", "Back"}
		selectBox := &survey.Select{Message: "Pick your process", Options: processes, PageSize: 20}
		err := survey.AskOne(selectBox, &process)
		if err != nil {
			log.Println(err)
		}

		if process == "List Domain" {
			listDomains(false)
		} else if process == "Create New Domain" {
			createDomain()
		} else if process == "Delete Domain" {
			deleteDomain()
		} else if process == "Renew Domain" {
			renewDomain()
		} else if process == "Start Tunnel" {
			startTunnel()
		} else if process == "Close Tunnel" {
			closeTunnel()
		}
	}
	if process != "Back" {
		tunnelProxy()
	}
}

func closeTunnel() {
	var list []string
	selectedList := listDomains(true)
	for _, item := range selectedList {
		list = append(list, item)
	}
	client.CloseTunnel(list)
}

func startTunnel() {
	domainList := client.ListDomain().Data.Domains
	selectedList := listDomains(true)
	var tunnels []models.Tunnel
	for _, selected := range selectedList {

		tunnels = append(tunnels, models.Tunnel{
			IndexId:       getDomainIndexId(selected, domainList),
			LocalPort:     askLocalPort(selected),
			DestinationIp: askDestinationIp(selected),
			DomainId:      getDomainIndexId(selected, domainList),
			LocalIp:       askLocalIp(selected),
			Domain:        selected,
		})
	}

	client.StartTunnel(tunnels, sshUser, sshPassword)
}

func askLocalIp(selected string) string {
	var localIp string
	survey.AskOne(&survey.Input{Message: "Local IP For " + selected + "(default: 127.0.0.1):"}, &localIp)
	if localIp == "" {
		localIp = "127.0.0.1"
	}
	return localIp
}

func getDomainId(selected string, list []models.DomainItem) int {
	for _, item := range list {
		if item.Domain == selected {
			return item.ID
		}
	}
	return 0
}

func askDestinationIp(selected string) string {
	var destinationIp string
	survey.AskOne(&survey.Input{Message: "Destination IP For " + selected + "(default: 127.0.0.1):"}, &destinationIp)
	if destinationIp == "" {
		destinationIp = "127.0.0.1"
	}
	return destinationIp
}

func askLocalPort(selected string) int {
	var localPort int
	survey.AskOne(&survey.Input{Message: "Local Port For " + selected + "(default: 80):"}, &localPort)
	if localPort == 0 {
		localPort = 80
	}
	return localPort
}

func getDomainIndexId(selected string, list []models.DomainItem) int {
	for indexId, item := range list {
		if item.Domain == selected {
			return indexId + 1
		}
	}
	return 0
}

func renewDomain() {
	selectedList := listDomains(false)
	client.RenewDomain(selectedList[0])
}

func deleteDomain() {
	selectedList := listDomains(true)
	selectedIdList := getDomainIdList(selectedList, client.ListDomain().Data.Domains)
	client.DeleteDomain(selectedIdList)
}

func createDomain() {
	var domain string
	survey.AskOne(&survey.Input{Message: "Domain Name/Empty Random:"}, &domain)
	response := client.CreateDomain(domain).(models.Response)

	var addVhost string
	survey.AskOne(&survey.Select{Message: "Add VHost?", Options: []string{"Y", "N"}}, &addVhost)
	if addVhost == "Y" {

		var service string
		var folder string
		var phpService string
		var typeConf string
		var proxyPass string

		service = pickService()

		if domain == "" {
			domain = response.Data["domain"].(string) + ".resoft.org"
		} else {
			domain = domain + ".resoft.org"
		}

		typeConf = "Default"

		if typeConf == "Default" {
			inputBox := &survey.Input{Message: "Folder:"}
			err := survey.AskOne(inputBox, &folder)
			if err != nil {
				log.Println(err)
			}

			phpService = selectPhpServices()
		}

		dockerEnvironmentManager.AddVirtualHost(service, domain, folder, phpService, typeConf, proxyPass, false)
	}
}

func listDomains(multiple bool) []string {
	var process string
	var multiProcesses []string

	domainList := client.ListDomain()
	var domains []string
	for _, domain := range domainList.Data.Domains {
		domains = append(domains, domain.Domain)
	}
	domains = append(domains, "Back")

	if multiple {
		selectBox := &survey.MultiSelect{Message: "Pick your process", Options: domains, PageSize: 20}
		survey.AskOne(selectBox, &multiProcesses)
		return multiProcesses
	} else {
		selectBox := &survey.Select{Message: "Pick your process", Options: domains, PageSize: 20}
		survey.AskOne(selectBox, &process)
		return []string{process}
	}
}

func getDomains(selects []string, domainList []models.DomainItem) []models.DomainItem {
	var list []models.DomainItem
	for _, selected := range selects {
		for _, domain := range domainList {
			if domain.Domain == selected {
				list = append(list, domain)
			}
		}
	}
	return list
}

func getDomainIdList(selects []string, domainList []models.DomainItem) []string {
	var list []string
	for _, selected := range selects {
		for _, domain := range domainList {
			if domain.Domain == selected {
				list = append(list, strconv.Itoa(domain.ID))
			}
		}
	}
	return list
}
