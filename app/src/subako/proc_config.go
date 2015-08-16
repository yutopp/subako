package subako

import (
	"log"
	"os"

	"sync"
	"errors"

	"sort"

	"path/filepath"
	"fmt"
	"strings"

)

// ProcConfig has
//   - PackageConfig
//   - LangConfig
type ProcConfigSetsConfig struct {
	IsRemote		bool
	BaseDir			string
	Repository		string
}


type targetPath string

func listHasPrefix(a, b []string) bool {
	for index, ap := range a {
		if index >= len(b) {
			return true
		}

		if ap != b[index] {
			return false
		}
	}

	return true
}

func globConfigPaths(genBaseDir string) ([]targetPath, error) {
	targets := make([]targetPath, 0, 100)
	tmpTargets := make([][]string, 0, 100)

	if err := filepath.Walk(genBaseDir, func(path string, info os.FileInfo, err error) error {
		if path == genBaseDir { return nil }
		if !info.IsDir() { return nil }

		// do not collect nested dirs
		splittedPath := strings.Split(filepath.Clean(path), string(filepath.Separator))
		hasPrefix := func() bool {
			for _, tmpPath := range tmpTargets {
				if listHasPrefix(splittedPath, tmpPath) {
					return true
				}
			}
			return false
		}()
		if hasPrefix {
			return nil
		}
		tmpTargets = append(tmpTargets, splittedPath)

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


type ProcConfigMap map[PackageName]*PackageBuildConfigSet

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
	paths, err := globConfigPaths(ctx.BaseDir)
	if err != nil {
		return err
	}
	log.Printf("package configs glob : %v", paths)

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
) (*PackageBuildConfig, error) {
	if _, ok := ctx.Map[PackageName(name)]; !ok {
		msg := fmt.Sprintf("There are no proc profiles for %s", name)
		return nil, errors.New(msg)
	}
	configSet := ctx.Map[PackageName(name)]

	if _, ok := configSet.Configs[PackageVersion(version)]; !ok {
		msg := fmt.Sprintf("%s has no proc profile for version %s", name, version)
		return nil, errors.New(msg)
	}

	return configSet.Configs[PackageVersion(version)], nil
}

func (ctx *ProcConfigSetsContext) FindWithDep(
	name, version			string,
	depName, depVersion		string,
	aps						*AvailablePackages,
) (*PackageBuildConfigWithDep, error) {
	pkgBuildConf, err := ctx.Find(name, version)
	if err != nil {
		return nil, err
	}

	ap, err := aps.Find(PackageName(depName), PackageVersion(depVersion))
	if err != nil {
		return nil, err
	}

	return &PackageBuildConfigWithDep{
		PackageBuildConfig: pkgBuildConf,
		DepAP: ap,
	}, nil
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


func (ctx *ProcConfigSetsContext) SortedConfigSets() []*PackageBuildConfigSet {
	var keys []string
    for k := range ctx.Map {
        keys = append(keys, string(k))
    }
    sort.Strings(keys)

	var sets []*PackageBuildConfigSet
	for _, name := range keys {
		sets = append(sets, ctx.Map[PackageName(name)])
	}

	return sets
}
