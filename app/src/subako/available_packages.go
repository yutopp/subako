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
	Name						PackageName
	Version						PackageVersion
	DisplayVersion				string

	GeneratedPackageFileName	string
	GeneratedPackageName		string
	GeneratedPackageVersion		string

	InstallBase					string
	InstallPrefix				string

	DepName						PackageName
	DepVersion					PackageVersion
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

	s = reVersion.ReplaceAllString(s, string(ap.Version))
	s = reDisplayVersion.ReplaceAllString(s, ap.DisplayVersion)

	//
	if fs := reUndef.FindString(s); fs != "" {
		return "", fmt.Errorf("Unknown placeholder %s is found in template", fs)
	}

	return s, nil
}

type AvailablePackagesDepVerMap map[PackageVersion]AvailablePackage
type AvailablePackagesDepNameMap map[PackageName]AvailablePackagesDepVerMap

type AvailablePackagesVerMap map[PackageVersion]AvailablePackagesDepNameMap

type AvailablePackages struct {
	LastUpdated		int64					// Unix time
	Packages		map[PackageName]AvailablePackagesVerMap

	FilePath		string		`json:"-"`	// ignore
	m				sync.Mutex	`json:"-"`	// ignore
}

func LoadAvailablePackages(path string) (*AvailablePackages, error) {
	if !Exists(path) {
		return &AvailablePackages{
			LastUpdated: 0,
			Packages: make(map[PackageName]AvailablePackagesVerMap),
			FilePath: path,
		}, nil
	}

	buffer, err := ioutil.ReadFile(path)
    if err != nil {
		return nil, err
    }

	var ap AvailablePackages
	if err := json.Unmarshal(buffer, &ap); err != nil {
		return nil, fmt.Errorf("%s : %v", path, err)
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


func (ap *AvailablePackages) fillNil(
	pkgName			PackageName,
	pkgVersion		PackageVersion,
	depPkgName		PackageName,
	depPkgVersion	PackageVersion,
) {
	if ap.Packages == nil {
		ap.Packages = make(map[PackageName]AvailablePackagesVerMap)
	}

	if _, ok := ap.Packages[pkgName]; !ok {
		ap.Packages[pkgName] = make(AvailablePackagesVerMap)
	}

	if _, ok := ap.Packages[pkgName][pkgVersion]; !ok {
		ap.Packages[pkgName][pkgVersion] = make(AvailablePackagesDepNameMap)
	}

	if _, ok := ap.Packages[pkgName][pkgVersion][depPkgName]; !ok {
		ap.Packages[pkgName][pkgVersion][depPkgName] = make(AvailablePackagesDepVerMap)
	}
}


func (ap *AvailablePackages) Update(
	a *AvailablePackage,
) error {
	ap.m.Lock()
	defer ap.m.Unlock()

	log.Printf("Update AvailablePackages => %v", *a)

	ap.fillNil(a.Name, a.Version, a.DepName, a.DepVersion)
	ap.Packages[a.Name][a.Version][a.DepName][a.DepVersion] = *a

	ap.LastUpdated = time.Now().Unix()

	log.Println("PACKAGES = ", ap)

	return nil
}


func (ap *AvailablePackages) Remove(
	name, version		string,
	depName, depVersion	string,
) error {
	ap.m.Lock()
	defer ap.m.Unlock()

	if ap.Packages == nil {
		return fmt.Errorf("There are no packages")
	}

	log.Printf("removing => %s / %s", name, version)

	if packages, ok := ap.Packages[PackageName(name)]; ok {
		// has 'name' key
		if depPkgMap, ok := packages[PackageVersion(version)]; ok {
			if depPkgVerMap, ok := depPkgMap[PackageName(depName)]; ok {
				if _, ok := depPkgVerMap[PackageVersion(depVersion)]; ok {
					delete(depPkgVerMap,PackageVersion(depVersion))
				}

				if len(depPkgVerMap) == 0 {
					delete(depPkgMap, PackageName(depName))
				}
			}

			if len(depPkgMap) == 0 {
				delete(packages, PackageVersion(version))
			}
		}

		if len(packages) == 0 {
			// there are no packages
			delete(ap.Packages, PackageName(name))
		}

	} else {
		return fmt.Errorf("There are no packages named %s", name)
	}

	ap.LastUpdated = time.Now().Unix()

	log.Println("PACKAGES", ap)

	return nil
}


func (ap *AvailablePackages) Find(
	name		PackageName,
	version		PackageVersion,
) (*AvailablePackage, error) {
	depName := PackageName("")
	depVersion := PackageVersion("")

	return ap.FindDep(name, version, depName, depVersion)
}

func (ap *AvailablePackages) FindDep(
	name		PackageName,
	version		PackageVersion,
	depName		PackageName,
	depVersion	PackageVersion,
) (*AvailablePackage, error) {
	ap.m.Lock()
	defer ap.m.Unlock()

	if ap.Packages == nil {
		return nil, fmt.Errorf("There are no available packages")
	}

	if packages, ok := ap.Packages[name]; ok {
		if depPkgMap, ok := packages[version]; ok {
			if depPkgVerMap, ok := depPkgMap[depName]; ok {
				if pkg, ok := depPkgVerMap[depVersion]; ok {
					return &pkg, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("(%s,%s)[with %s, %s] is not found in available packages", name, version, depName, depVersion)
}


type APWalkFunc func(name PackageName, version PackageVersion, depName PackageName, depVersion PackageVersion, ap *AvailablePackage) error
func (ap *AvailablePackages) Walk(f APWalkFunc) error {
	for name, packages := range ap.Packages {
		for version, depPkgMap := range packages {
			for depName, depPkgVerMap := range depPkgMap {
				for depVersion, pkg := range depPkgVerMap {
					if err := f(name, version, depName, depVersion, &pkg); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}
