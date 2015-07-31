package subako

import (
	"log"
	"os"
	"os/exec"
	"sync"
	"errors"
	"bytes"
	"sort"

	"encoding/json"

	"path"
	"path/filepath"
	"fmt"
	"strings"

	"io/ioutil"
)


type ProcConfigSetsConfig struct {
	IsRemote		bool
	BaseDir			string
	Repository		string
}


// TODO:
// if version contains charactors sush as '/', make them error
type ProcConfig struct {
	parentTask			*ProcConfigSet
	name				string
	version				string
	targetSystem		string		// Ex. x86_64-linux-gnu
	targetArch			string		// Ex. x86_64
	basePath			string
	dependend			*ProfileTemplate
}



func (tc *ProcConfig) makeWorkDirName() string {
	return tc.name + "-" + tc.targetSystem + "-" + tc.version
}

func (tc *ProcConfig) makePackagePathName() string {
	return tc.name + "." + tc.version
}


type ProcConfigSetJSON struct {
	Name		string		`json:"name"`
	Versions	[]string	`json:"versions"`
	Type		string		`json:"type"`
}

type ProcConfigSet struct {
	Name				string
	Type				string
	VersionedConfigs	map[string]*ProcConfig	// map[version]
	ProfileTemplate		*ProfileTemplate
	ProfilePatches		[]*ProfilePatch
}

func (pc *ProcConfigSet) SortedConfigs() []*ProcConfig {
	var keys []string
    for k := range pc.VersionedConfigs {
        keys = append(keys, k)
    }
    sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	var confs []*ProcConfig
	for _, version := range keys {
		confs = append(confs, pc.VersionedConfigs[version])
	}

	return confs
}


func makeProcConfigSet(baseDir targetPath) (*ProcConfigSet, error) {
	configPath := path.Join(string(baseDir), "config.json")
	log.Println("config path", configPath);

	file, err := ioutil.ReadFile(configPath)
    if err != nil {
        return nil, err
    }

	var c ProcConfigSetJSON
	if err := json.Unmarshal(file, &c); err != nil {
		return nil, err
	}

	if len(c.Name) == 0 {
		panic("name")	// TODO: fix
	}

	if c.Versions == nil || len(c.Versions) == 0 {
		panic("versions")	// TODO: fix
	}

	// return value
	configSet := &ProcConfigSet{
		Name: c.Name,
		Type: c.Type,
	}

	// read config
	versionedConfigs := make(map[string]*ProcConfig)
	for _, version := range c.Versions {
		config := &ProcConfig{
			parentTask: configSet,
			name: c.Name,
			version: version,
			targetSystem: "x86_64-linux-gnu",	// tmp
			targetArch: "x86_64",				// tmp
			basePath: string(baseDir),
			dependend: nil,
		}

		versionedConfigs[version] = config
	}

	ptBasePath := filepath.Join(string(baseDir), "profile_templates")

	// read profile templates
	ptPath := filepath.Join(ptBasePath, "template.yml")
	var profileTemplate *ProfileTemplate = nil
	if Exists(ptPath) {
		pt, err := readProfileTemplate(ptPath)
		if err != nil {
			return nil, fmt.Errorf("%v at %s", err, ptPath)
		}
		profileTemplate = pt
	}

	// read profile patch
	patches := make([]*ProfilePatch, 0)
	if err := filepath.Walk(ptBasePath, func(path string, info os.FileInfo, err error) error {
		if path == ptBasePath { return nil }
		if info.IsDir() { return nil }
		// do not collect files that name is started by _ or .
		if strings.HasPrefix(filepath.Base(path), "_") {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}

		//
		if strings.HasPrefix(filepath.Base(path), "patch_") {
			pt, err := readProfilePatch(path)
			if err != nil {
				return fmt.Errorf("%v at %s", err, path)
			}
			patches = append(patches, pt)
			return nil
		}

		return nil

	}); err != nil {
		return nil, err
	}

	log.Printf("Results: %v\n", c)

	// update
	configSet.VersionedConfigs = versionedConfigs
	configSet.ProfileTemplate = profileTemplate
	configSet.ProfilePatches = patches

	return configSet, nil
}

type targetPath string

func globProcConfigPaths(genBaseDir string) ([]targetPath, error) {
	targets := make([]targetPath, 0, 100)
	tmpTargets := make([]string, 0, 100)

	if err := filepath.Walk(genBaseDir, func(path string, info os.FileInfo, err error) error {
		if path == genBaseDir { return nil }
		if !info.IsDir() { return nil }

		// do not collect nested dirs
		hasPrefix := func() bool {
			for _, v := range tmpTargets {
				if strings.HasPrefix(filepath.Clean(path), filepath.Clean(v)) {
					return true
				}
			}
			return false
		}()
		tmpTargets = append(tmpTargets, path)
		if hasPrefix {
			return nil
		}

		// do not collect dirs that name is started by _ or .
		if strings.HasPrefix(filepath.Base(path), "_") {
			return nil
		}

		if strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}

		log.Printf("=> %s", filepath.Base(path))
		targets = append(targets, targetPath(path))
		return nil;

	}); err != nil {
		return nil, err
	}

	return targets, nil
}


type gitRepository struct {
	BaseDir			string
	Url				string

	Revision		string
}

func (g *gitRepository) Clone() error {
	cmd := exec.Command("git", "clone", g.Url, g.BaseDir)
	if err := cmd.Run(); err != nil {
		return err
	}

	g.GetRevision()		// no error check

	return nil
}

func (g *gitRepository) Pull() error {

	if g.Revision != "" {
		cmd := exec.Command("bash", "-c", fmt.Sprintf("cd '%s' && git reset --hard origin/master", g.BaseDir))
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out

		if err := cmd.Run(); err != nil {
			log.Printf("Error: git reset --hard origin/master\n%s\n", out.String())
			return err
		}

		log.Printf("git reset --hard origin/master\n%s\n", out.String())
	}

	{
		cmd := exec.Command("bash", "-c", fmt.Sprintf("cd '%s' && git pull origin master", g.BaseDir))
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out

		if err := cmd.Run(); err != nil {
			log.Printf("Error: git pull origin master\n%s\n", out.String())
			return err
		}

		log.Printf("git pull origin master\n%s\n", out.String())
	}

	return g.GetRevision()
}

func (g *gitRepository) GetRevision() error {
	cmd := exec.Command("bash", "-c", fmt.Sprintf("cd '%s' && git rev-parse HEAD", g.BaseDir))
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return err
	}

	hash := out.String()
	log.Printf("commit hash: %s", hash)

	g.Revision = hash

	return nil
}


type ProcConfigMap map[string]*ProcConfigSet	// map[name]

type ProcConfigSetsContext struct{
	BaseDir			string
	IsRemote		bool
	Repo			*gitRepository

	Map				ProcConfigMap

	m				sync.Mutex
}

func MakeProcConfigSetsContext(
	config	*ProcConfigSetsConfig,
) (*ProcConfigSetsContext, error) {
	procConfigSetsCtx := &ProcConfigSetsContext{
		BaseDir: config.BaseDir,
		IsRemote: config.IsRemote,
	}

	if config.IsRemote {
		procConfigSetsCtx.Repo = &gitRepository{
			BaseDir: config.BaseDir,
			Url: config.Repository,
		}
	}

	//
	if !Exists(procConfigSetsCtx.BaseDir) {
		if config.IsRemote {
			if err := procConfigSetsCtx.Repo.Clone(); err != nil {
				return nil, err
			}

		} else {
			return nil, errors.New("ConfigSets basedir is not found")
		}
	}

	if err := procConfigSetsCtx.Update(); err != nil {
		return nil, err
	}

	return procConfigSetsCtx, nil
}

func (ctx *ProcConfigSetsContext) Glob() error {
	newMap := make(ProcConfigMap)

	//
	paths, err := globProcConfigPaths(ctx.BaseDir)
	if err != nil {
		return err
	}
	log.Println(paths)

	for _, v := range paths {
		tc, err := makeProcConfigSet(v)
		if err != nil {
			return err
		}
		newMap[tc.Name] = tc
	}

	// update
	ctx.Map = newMap

	return nil
}

func (ctx *ProcConfigSetsContext) Find(
	name, version		string,
) (*ProcConfig, error) {
	if _, ok := ctx.Map[name]; !ok {
		msg := fmt.Sprintf("There are no proc profiles for %s", name)
		return nil, errors.New(msg)
	}
	configSet := ctx.Map[name]

	if _, ok := configSet.VersionedConfigs[version]; !ok {
		msg := fmt.Sprintf("%s has no proc profile for version %s", name, version)
		return nil, errors.New(msg)
	}

	return configSet.VersionedConfigs[version], nil
}

func (ctx *ProcConfigSetsContext) Update() error {
	if ctx.IsRemote {
		if err := ctx.Repo.Pull(); err != nil {
			return err
		}
	}

	if err := ctx.Glob(); err != nil {
		return err
	}

	return nil
}


func (ctx *ProcConfigSetsContext) SortedConfigSets() []*ProcConfigSet {
	var keys []string
    for k := range ctx.Map {
        keys = append(keys, k)
    }
    sort.Strings(keys)

	var sets []*ProcConfigSet
	for _, name := range keys {
		sets = append(sets, ctx.Map[name])
	}

	return sets
}
