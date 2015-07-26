package subako

import (
	"encoding/json"
	"regexp"
	"time"
	"io/ioutil"
	"log"
	"sync"
)


type AvailablePackage struct {
	Version				string
	PackageFileName		string
	PackageName			string
	PackageVersion		string
	DisplayVersion		string

	InstallBase			string
}

func (ap *AvailablePackage) ReplaceString(s string) string {
	re1 := regexp.MustCompile("%{install_base}")
	re2 := regexp.MustCompile("%{version}")
	re3 := regexp.MustCompile("%{display_version}")

	s1 := re1.ReplaceAllString(s, ap.InstallBase)
	s2 := re2.ReplaceAllString(s1, ap.Version)
	s3 := re3.ReplaceAllString(s2, ap.DisplayVersion)

	return s3
}


type AvailableVerPackages map[string]AvailablePackage		// map[Version]

type AvailablePackages struct {
	LastUpdated		int64		// Unix time
	Packages		map[string]AvailableVerPackages	// map[Name]detail

	FilePath		string		`json:"-"`	// ignore
	m				sync.Mutex	`json:"-"`	// ignore
}

func LoadAvailablePackages(path string) (*AvailablePackages, error) {
	if !exists(path) {
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
