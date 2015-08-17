package subako

import (
	"log"
	"sort"
	"fmt"
	"encoding/json"
	"path"
	"io/ioutil"
)


type PackageName		string
type PackageVersion		string


type IPackageBuildConfig interface {
	GetName() PackageName
	GetVersion() PackageVersion
	GetTargetSystem() string
	GetTargetArch() string

	GetBasePath() string

	makeWorkDirName() string
	makePackagePathName() string

	GetDepName() PackageName
	GetDepVersion() PackageVersion
	GetGenPkgName() string
	GetDepPackage() *AvailablePackage

	GetRepDeps() []PackageName
}

// Unit
// TODO:
// if version contains charactors sush as '/', make them error
type PackageBuildConfig struct {
	name				string
	version				string
	targetSystem		string		// Ex. x86_64-linux-gnu
	targetArch			string		// Ex. x86_64
	basePath			string

	refDeps				[]PackageName
}

func (tc *PackageBuildConfig) makeWorkDirName() string {
	return tc.name + "-" + tc.targetSystem + "-" + tc.version
}

func (tc *PackageBuildConfig) makePackagePathName() string {
	return tc.name + "." + tc.version
}

func (tc *PackageBuildConfig) GetName() PackageName { return PackageName(tc.name) }
func (tc *PackageBuildConfig) GetVersion() PackageVersion { return PackageVersion(tc.version) }
func (tc *PackageBuildConfig) GetTargetSystem() string { return tc.targetSystem }
func (tc *PackageBuildConfig) GetTargetArch() string { return tc.targetArch }
func (tc *PackageBuildConfig) GetBasePath() string { return tc.basePath }
func (tc *PackageBuildConfig) GetDepName() PackageName { return PackageName("") }
func (tc *PackageBuildConfig) GetDepVersion() PackageVersion { return PackageVersion("") }
func (tc *PackageBuildConfig) GetGenPkgName() string { return tc.name }
func (tc *PackageBuildConfig) GetDepPackage() *AvailablePackage { return nil }

func (tc *PackageBuildConfig) GetRepDeps() []PackageName { return tc.refDeps }


//
type PackageBuildConfigWithDep struct {
	*PackageBuildConfig
	DepAP		*AvailablePackage
}

func (tc *PackageBuildConfigWithDep) makeWorkDirName() string {
	return fmt.Sprintf("%s-%s-%s-with-%s-%s", tc.name, tc.targetSystem, tc.version, tc.DepAP.Name, tc.DepAP.Version)
}

func (tc *PackageBuildConfigWithDep) makePackagePathName() string {
	return fmt.Sprintf("%s.%s<with.%s.%s>", tc.name, tc.version, tc.DepAP.Name, tc.DepAP.Version)
}

func (tc *PackageBuildConfigWithDep) GetDepName() PackageName {
	return tc.DepAP.Name
}
func (tc *PackageBuildConfigWithDep) GetDepVersion() PackageVersion {
	return tc.DepAP.Version
}
func (tc *PackageBuildConfigWithDep) GetGenPkgName() string {
	return fmt.Sprintf("%s--with-%s.%s-", tc.name, tc.DepAP.Name, tc.DepAP.Version)
}
func (tc *PackageBuildConfigWithDep) GetDepPackage() *AvailablePackage { return tc.DepAP }


// Set
type PackageBuildConfigSet struct {
	Name				PackageName			`json:"name"`
	Versions			[]PackageVersion	`json:"versions"`
	Type				string				`json:"type"`
	QueueWith			[]PackageName		`json:"queue_with"`

	DepPkgs				map[PackageName][]PackageVersion	`json:"dep_pkgs"`

	Configs				map[PackageVersion]*PackageBuildConfig

	LangConfigs			map[LanguageName]*LangConfigSet
}

func (pc *PackageBuildConfigSet) SortedConfigs() []*PackageBuildConfig {
	var keys []string
    for k := range pc.Configs {
        keys = append(keys, string(k))
    }
    sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	var confs []*PackageBuildConfig
	for _, version := range keys {
		confs = append(confs, pc.Configs[PackageVersion(version)])
	}

	return confs
}


func (pc *PackageBuildConfigSet) SortedLangConfigs() []*LangConfigSet {
	var keys []string
    for k := range pc.LangConfigs {
        keys = append(keys, string(k))
    }
    sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	var confs []*LangConfigSet
	for _, name := range keys {
		confs = append(confs, pc.LangConfigs[LanguageName(name)])
	}

	return confs
}


type SDepPkg struct{
	Name	PackageName
	Version	PackageVersion
}
func (pc *PackageBuildConfigSet) SortedDepPkgs() []SDepPkg {
	var keys []string
    for k := range pc.DepPkgs {
        keys = append(keys, string(k))
    }
    sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	var confs []SDepPkg
	for _, name := range keys {
		vers := pc.DepPkgs[PackageName(name)]
		for _, ver := range vers {
			confs = append(confs, SDepPkg{
				Name: PackageName(name),
				Version: ver,
			})
		}
	}

	return confs
}


func makeProcConfigSet(baseDir targetPath) (*PackageBuildConfigSet, error) {
	configPath := path.Join(string(baseDir), "package_config.json")
	log.Println("package config path", configPath);

	file, err := ioutil.ReadFile(configPath)
    if err != nil {
        return nil, err
    }

	configSet := &PackageBuildConfigSet{
		Configs: make(map[PackageVersion]*PackageBuildConfig),
		LangConfigs: make(map[LanguageName]*LangConfigSet),
	}
	if err := json.Unmarshal(file, configSet); err != nil {
		return nil, err
	}

	if len(configSet.Name) == 0 {
		panic("name")	// TODO: fix
	}

	if configSet.Versions == nil || len(configSet.Versions) == 0 {
		panic("versions")	// TODO: fix
	}

	if !Exists(path.Join(string(baseDir), "install.sh")) {
		// TODO: error check...?
	}

	// read config
	for _, version := range configSet.Versions {
		config := &PackageBuildConfig{
			name: string(configSet.Name),
			version: string(version),
			targetSystem: "x86_64-linux-gnu",	// tmp
			targetArch: "x86_64",				// tmp
			basePath: string(baseDir),

			refDeps: configSet.QueueWith,
		}

		configSet.Configs[version] = config
	}

	//
	langConfigPaths, err := globConfigPaths(string(baseDir))
	if err != nil {
		return nil, err
	}
	log.Printf("lang configs glob : %v", langConfigPaths)
	for _, p := range langConfigPaths {
		config, err := makeLangConfigSet(p)
		if err != nil {
			return nil, err
		}
		configSet.LangConfigs[config.Name] = config;
	}


	return configSet, nil
}
