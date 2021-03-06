package main

import (
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

// Installed detects whether the OS is using dpkg, rpm, or pacman, queries
// a package accoringly, and returns an error if it is not installed.
func Installed(pkg string) Thunk {
	// getManager returns the program to use for the query
	getManager := func(managers []string) string {
		for _, program := range managers {
			cmd := exec.Command(program, "--version")
			err := cmd.Start()
			// as long as the command was found, return that manager
			message := ""
			if err != nil {
				message = err.Error()
			}
			if strings.Contains(message, "not found") == false {
				return program
			}
		}
		log.Fatal("No package manager found. Attempted: " + fmt.Sprint(managers))
		return "" // never reaches this return
	}

	// package managers and their options
	managers := map[string]string{
		"dpkg":   "-s",
		"rpm":    "-q",
		"pacman": "-Qs",
	}
	keys := make([]string, len(managers))
	i := 0
	for key := range managers {
		keys[i] = key
		i++
	}

	return func() (exitCode int, exitMessage string) {
		name := getManager(keys)
		options := managers[name]
		out, _ := exec.Command(name, options, pkg).Output()
		if strings.Contains(string(out), pkg) {
			return 0, ""
		}
		msg := "Package was not found:"
		msg += "\n\tPackage name: " + pkg
		msg += "\n\tPackage manager: " + name
		return 1, msg
	}
}

// PPA checks to see whether a given PPA is enabled on Ubuntu-based systems
func PPA(name string) Thunk {
	// getAptSources returns all the urls of all apt sources (including source
	// code repositories
	getAptSources := func(path string) (urls []string) {
		split := stringToSlice(fileToString(path))
		// filter out comments
		commentRegex := regexp.MustCompile("^\\s*#.*")
		for _, line := range split {
			if len(line) > 1 && !(commentRegex.MatchString(line[0])) {
				urls = append(urls, line[1])
			}
		}
		return urls
	}
	// getPPAs returns a list of all PPAs in sources.list (as URLs)
	getPPAs := func(path string) (ppas []string) {
		for _, url := range getAptSources(path) {
			if strings.Contains(url, "ppa") {
				ppas = append(ppas, url)
			}
		}
		return ppas
	}
	// valid URL uses net/url's Parse function to determine if the given url
	// was indeed valid
	validURL := func(urlstr string) bool {
		_, err := url.Parse(urlstr)
		if err == nil {
			return true
		}
		return false
	}
	return func() (exitCode int, exitMessage string) {
		ppas := getPPAs("/etc/apt/sources.list")
		for _, ppa := range ppas {
			if !validURL(ppa) {
				return 1, "PPA URL invalid: " + ppa
			} else if strings.Contains(ppa, name) {
				return 0, ""
			}
		}
		return genericError("PPA not found", name, ppas)
	}
}

type YumRepo struct {
	Name, Fullname, Url string
}

// getYumRepos returns a list of Yum Repos taken from /etc/yum.conf
func getYumRepos(path string) (repos []YumRepo) {
	var fullNames []string
	var urls []string
	commentRegex := regexp.MustCompile("^\\s*#.*")
	for _, line := range fileToLines(path) {
		// filter comments and convert to string
		strLine := string(line)
		if !(commentRegex.Match(line)) {
			// first, attempt to replace the prefix
			replaceName := strings.TrimPrefix(strLine, "name=")
			replaceURL := strings.TrimPrefix(strLine, "baseurl=")
			// if they are different, we know a prefix was replaced
			if replaceName != strLine {
				fullNames = append(fullNames, replaceName)
			} else if replaceURL != strLine {
				urls = append(urls, replaceURL)
			}
		}
	}
	// Get shortest list to zip with, so we don't get an index error
	shortList := fullNames
	if len(fullNames) > len(urls) {
		shortList = urls
	}
	// Construct YumRepos
	whitespaceRegex := regexp.MustCompile("\\s+")
	for i, _ := range shortList {
		nameSplit := whitespaceRegex.Split(fullNames[i], -1)
		shortName := nameSplit[len(nameSplit)-1]
		repo := YumRepo{Name: shortName, Fullname: fullNames[i], Url: urls[i]}
		repos = append(repos, repo)
	}
	return repos
}

// existsRepoWithProperty is an abstraction of YumRepoExists and YumRepoURL.
// It takes a struct field name to check, and an expected value. If the expected
// value is found in the field of a repo, it returns 0, "" else an error message.
// Valid choices for prop: "Url" | "Name" | "Fullname"
func existsRepoWithProperty(prop string, val string) (int, string) {
	var properties []string
	for _, repo := range getYumRepos("/etc/yum.conf") {
		switch prop {
		case "Url":
			properties = append(properties, repo.Url)
		case "Name":
			properties = append(properties, repo.Name)
		case "Fullname":
			properties = append(properties, repo.Fullname)
		default:
			log.Fatal("Yum repos don't have the requested property: " + prop)
		}
	}
	if strIn(val, properties) {
		return 0, ""
	}
	msg := "Yum repo with given " + prop + " not found"
	return genericError(msg, val, properties)
}

// YumRepo checks to see that a given yum repo is currently active
func YumRepoExists(name string) Thunk {
	return func() (exitCode int, exitMessage string) {
		return existsRepoWithProperty("Name", name)
	}
}

// YumRepoURL checks to see if the Yum repo with the given URL is active
func YumRepoURL(urlstr string) Thunk {
	return func() (exitCode int, exitMessage string) {
		return existsRepoWithProperty("Url", urlstr)
	}
}

// pacmanIgnore checks to see whether a given package is in /etc/pacman.conf's
// IgnorePkg setting
func pacmanIgnore(pkg string) Thunk {
	return func() (exitCode int, exitMessage string) {
		data := fileToString("/etc/pacman.conf")
		re := regexp.MustCompile("[^#]IgnorePkg\\s+=\\s+.+")
		find := re.FindString(data)
		var packages []string
		if find != "" {
			spl := strings.Split(find, " ")
			if len(spl) > 2 {
				packages = spl[2:] // first two are "IgnorePkg" and "="
				if strIn(pkg, packages) {
					return 0, ""
				}
			}
		}
		msg := "Couldn't find package in IgnorePkg"
		return genericError(msg, pkg, packages)
	}
}
