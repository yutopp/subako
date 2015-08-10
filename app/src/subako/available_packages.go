package subako

import (
	"encoding/json"
	"regexp"
	"time"
	"io/ioutil"
	"log"
	"fmt"
	"sync"
)


type AvailablePackage struct {
	Version				string
	PackageFileName		string
	PackageName			string
	PackageVersion		string
	DisplayVersion		string

	InstallBase			string
	InstallPrefix		string
}

var (
	reInstallBase = regexp.MustCompile("%{install_base}")
	reInstallPrefix = regexp.MustCompile("%{install_prefix}")
	reVersion = regexp.MustCompile("%{version}")
	reDisplayVersion = regexp.MustCompile("%{display_version}")

	reUndef = regexp.MustCompile("%{.*}")
)

func (ap *AvailablePackage) ReplaceString(s string) (string, error) {
	s = reInstallBase.ReplaceAllString(s, ap.InstallBase)
	s = reInstallPrefix.ReplaceAllString(s, ap.InstallPrefix)

	s = reVersion.ReplaceAllString(s, ap.Version)
	s = reDisplayVersion.ReplaceAllString(s, ap.DisplayVersion)

	//
	if fs := reUndef.FindString(s); fs != "" {
		return "", fmt.Errorf("Unknown placeholder %s is found in template", fs)
	}

	return s, nil
}


type AvailableVerPackages map[string]AvailablePackage		// map[Version]

type AvailablePackages struct {
	LastUpdated		int64							// Unix time
	Packages		map[string]AvailableVerPackages	// map[Name]detail

	FilePath		string		`json:"-"`	// ignore
	m				sync.Mutex	`json:"-"`	// ignore
}

func LoadAvailablePackages(path string) (*AvailablePackages, error) {
	if !Exists(path) {
		return &AvailablePackages{
			LastUpdated: 0,
			Packages: make(map[string]AvailableVerPackages),
			FilePath: path,
		}, nil
	}

	buffer, err := ioutil.ReadFile(path)
    if err != nil {
		return nil, err
    }

	var ap AvailablePackages
	if err := json.Unmarshal(buffer, &ap); err != nil {
		return nil, err
	}
	ap.FilePath = path

	return &ap, nil
}

func (ap *AvailablePackages) Save() error {
	ap.m.Lock()
	defer ap.m.Unlock()

	buffer, err := json.Marshal(ap)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(ap.FilePath, buffer, 0644); err != nil {
		return err
	}

	return nil
}

func (ap *AvailablePackages) Update(name string, a *AvailablePackage) error {
	ap.m.Lock()
	defer ap.m.Unlock()

	if ap.Packages == nil {
		ap.Packages = make(map[string]AvailableVerPackages)
	}

	log.Printf("available => %v\n", *a)

	if _, ok := ap.Packages[name]; ok {
		// has key
		ap.Packages[name][a.Version] = *a

	} else {
		ap.Packages[name] = AvailableVerPackages{
			a.Version: *a,
		}
	}

	ap.LastUpdated = time.Now().Unix()

	log.Println("PACKAGES", ap)

	return nil
}


func (ap *AvailablePackages) Remove(name, version string) error {
	ap.m.Lock()
	defer ap.m.Unlock()

	if ap.Packages == nil {
		ap.Packages = make(map[string]AvailableVerPackages)
	}

	log.Printf("removing => %s / %s", name, version)

	if packages, ok := ap.Packages[name]; ok {
		// has 'name' key
		if _, ok := packages[version]; ok {
			// has 'version' key
			delete(packages, version)
		}

		if len(packages) == 0 {
			// there are no packages
			delete(ap.Packages, name)
		}

	} else {
		return fmt.Errorf("There are no packages named %s", name)
	}

	ap.LastUpdated = time.Now().Unix()

	log.Println("PACKAGES", ap)

	return nil
}
