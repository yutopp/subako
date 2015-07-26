package subako

import (
	"log"
	"os"

	"encoding/json"
	"gopkg.in/yaml.v2"

	"path"
	"path/filepath"
	"fmt"
	"strings"

	"io/ioutil"
)


func readProfileTemplate(
	filePath string,
) (*ProfileTemplate, error) {
	buf, err := ioutil.ReadFile(filePath)
    if err != nil {
        fmt.Printf("File error: %v\n", err)
        os.Exit(1)
    }

	var pt ProfileTemplate
	if err := yaml.Unmarshal(buf, &pt); err != nil {
		panic(err)
	}

	fmt.Println("PT", pt)

	return &pt, nil
}

func readProfilePatch(
	filePath string,
) (*ProfilePatch, error) {
	fmt.Println("patch F=> ",filePath)

	buf, err := ioutil.ReadFile(filePath)
    if err != nil {
        fmt.Printf("File error: %v\n", err)
        os.Exit(1)
    }

	var pt ProfilePatch
	if err := yaml.Unmarshal(buf, &pt); err != nil {
		panic(err)
	}

	fmt.Println("patch", pt)

	return &pt, nil
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



type IProfileTemplate interface {
	Generate(*Profile, *AvailablePackage)
}




/*
func replace() {
	re := regexp.MustCompile("#{display_version}")
	version
	install_base
	fmt.Println(re.ReplaceAllString("-ab-a#{x}xb-", "T"))
}
*/

type ProfilePatchFrom struct {
	Versions			[]string
}

type ProfilePatchTo struct {
	Name				string
	Versions			[]string
}

type ProfilePatch struct {
	From				*ProfilePatchFrom
	To					*ProfilePatchTo
	Append				struct {
		Compile				*ExecProfileTemplate
		Link				*ExecProfileTemplate
		Run					*ExecProfileTemplate
	}
}

func appendStringMap(recv, inj map[string]string) map[string]string {
	if inj == nil { return nil }
	result := make(map[string]string)
	for k, v := range recv {
		result[k] = v
	}

	for k, v := range inj {
		if _, ok := result[k]; ok {
			// append
			result[k] = result[k] + v
		} else {
			result[k] = v
		}
	}

	return result
}

func appendStringArrayMap(recv, inj map[string][]string) map[string][]string {
	if inj == nil { return nil }
	result := make(map[string][]string)
	for k, v := range recv {
		result[k] = v
	}

	for k, v := range inj {
		if _, ok := result[k]; ok {
			// append
			result[k] = append(result[k], v...)
		} else {
			result[k] = v
		}
	}

	return result
}

func (patch *ProfilePatch) Generate(
	profile *Profile,
	ap *AvailablePackage,
) {
	fmt.Printf("GENERATE by %s %s\n", ap.PackageName, ap.Version)

	profile.Compile = appendExecProfile(ap, profile.Compile, patch.Append.Compile)
	profile.Link = appendExecProfile(ap, profile.Link, patch.Append.Link)
	profile.Run = appendExecProfile(ap, profile.Run, patch.Append.Run)
}

func transformStringArray(gen []string, f func(string) string) []string {
	result := make([]string, len(gen))
	for i, v := range gen {
		result[i] = f(v)
	}
	return result
}

func transformStringNestedArray(gen [][]string, f func(string) string) [][]string {
	result := make([][]string, len(gen))
	for i, v := range gen {
		result[i] = transformStringArray(v, f)
	}
	return result
}

func transformStringMap(gen map[string]string, f func(string) string) map[string]string {
	result := make(map[string]string)
	for k, v := range gen {
		result[k] = f(v)
	}
	return result
}

func transformStringArrayMap(gen map[string][]string, f func(string) string) map[string][]string {
	result := make(map[string][]string)
	for k, v := range gen {
		result[k] = transformStringArray(v, f)
	}
	return result
}

type ExecProfileTemplate struct {
	Extension			string					`yaml:"extension,omitempty"`
	Commands			[]string				`yaml:"commands"`
	Envs				map[string]string		`yaml:"envs,omitempty"`
	FixedCommands		[][]string				`yaml:"fixed_commands,omitempty"`
	SelectableCommands	map[string][]string		`yaml:"selectable_commands,omitempty"`
}

func (ept *ExecProfileTemplate) String() string {
	if ept == nil { return "<nil>" }

	return fmt.Sprintf("(Extension: %s / ", ept.Extension) +
		fmt.Sprintf("Commands: %s / ", ept.Commands) +
		fmt.Sprintf("Envs: %s / ", ept.Envs) +
		fmt.Sprintf("FixedCommands: %s / ", ept.FixedCommands) +
		fmt.Sprintf("SelectableCommands: %s)", ept.SelectableCommands)
}

type ProfileTemplate struct {
	DisplayVersion		string					`yaml:"display_version"`
	IsBuildRequired		bool					`yaml:"is_build_required"`
	IsLinkIndependent	bool					`yaml:"is_link_independent"`

	Compile				*ExecProfileTemplate	`yaml:"compile,omitempty"`
	Link				*ExecProfileTemplate	`yaml:"link"`
	Run					*ExecProfileTemplate	`yaml:"run"`
}

func (template *ProfileTemplate) Generate(
	profile *Profile,
	ap *AvailablePackage,
) {
	fmt.Printf("GENERATE by %s %s\n", ap.PackageName, ap.Version)

	profile.DisplayVersion = ap.ReplaceString(template.DisplayVersion)
	profile.IsBuildRequired = template.IsBuildRequired
	profile.IsLinkIndependent = template.IsLinkIndependent

	profile.Compile = setExecProfile(ap, template.Compile)
	profile.Link = setExecProfile(ap, template.Link)
	profile.Run = setExecProfile(ap, template.Run)
}

func setExecProfile(
	ap *AvailablePackage,
	src *ExecProfileTemplate,
) *ExecProfile {
	if src == nil { return nil }

	var execProfile ExecProfile
	execProfile.Extension = ap.ReplaceString(src.Extension)
	execProfile.Commands = transformStringArray(src.Commands, ap.ReplaceString)
	execProfile.Envs = transformStringMap(src.Envs, ap.ReplaceString)
	execProfile.FixedCommands = transformStringNestedArray(src.FixedCommands, ap.ReplaceString)
	execProfile.SelectableCommands = transformStringArrayMap(src.SelectableCommands, ap.ReplaceString)

	return &execProfile
}

func appendExecProfile(
	ap *AvailablePackage,
	base *ExecProfile,
	src *ExecProfileTemplate,
) *ExecProfile {
	if src == nil { return nil }

	var execProfile ExecProfile = *base
	execProfile.Commands = append(
		base.Commands,
		transformStringArray(src.Commands, ap.ReplaceString)...
	)
	execProfile.Envs = appendStringMap(
		base.Envs,
		transformStringMap(src.Envs, ap.ReplaceString),
	)
	execProfile.FixedCommands = append(
		base.FixedCommands,
		transformStringNestedArray(src.FixedCommands, ap.ReplaceString)...
	)
	execProfile.SelectableCommands = appendStringArrayMap(
		base.SelectableCommands,
		transformStringArrayMap(src.SelectableCommands, ap.ReplaceString),
	)

	return &execProfile
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

func makeProcConfigSet(baseDir targetPath) *ProcConfigSet {
	configPath := path.Join(string(baseDir), "config.json")
	fmt.Println(configPath);

	file, err := ioutil.ReadFile(configPath)
    if err != nil {
        fmt.Printf("File error: %v\n", err)
        os.Exit(1)
    }

	fmt.Println(file)

	var c ProcConfigSetJSON
	if err := json.Unmarshal(file, &c); err != nil {
		fmt.Printf("File error: %v\n", err)
        os.Exit(1)
	}

	if len(c.Name) == 0 {
		panic("name")
	}

	if c.Versions == nil || len(c.Versions) == 0 {
		panic("versions")
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
			targetSystem: "x86_64-linux-gnu",
			targetArch: "x86_64",
			basePath: string(baseDir),
			dependend: nil,
		}

		versionedConfigs[version] = config
	}


	ptBasePath := filepath.Join(string(baseDir), "profile_templates")

	// read profile templates
	ptPath := filepath.Join(ptBasePath, "template.yml")
	var profileTemplate *ProfileTemplate = nil
	if exists(ptPath) {
		pt, err := readProfileTemplate(ptPath)
		if err != nil { panic("a") }
		profileTemplate = pt
	}

	// read profile patch
	patches := make([]*ProfilePatch, 0)
	if err := filepath.Walk(ptBasePath, func(path string, info os.FileInfo, err error) error {
		if path == ptBasePath { return nil }
		if info.IsDir() { return nil }
		// do not collect files that name is started by _
		if strings.HasPrefix(filepath.Base(path), "_") {
			return nil
		}

		if strings.HasPrefix(filepath.Base(path), "patch_") {
			pt, err := readProfilePatch(path)
			if err != nil { panic("a") }
			patches = append(patches, pt)
			return nil
		}

		return nil

	}); err != nil {
		panic("aaa")
	}

	fmt.Printf("Results: %v\n", c)

	// update
	configSet.VersionedConfigs = versionedConfigs
	configSet.ProfileTemplate = profileTemplate
	configSet.ProfilePatches = patches

	return configSet
}

type targetPath string

func globProcConfigPaths(genBaseDir string) ([]targetPath, error) {
	targets := make([]targetPath, 0)

	if err := filepath.Walk(genBaseDir, func(path string, info os.FileInfo, err error) error {
		if path == genBaseDir { return nil }
		if !info.IsDir() { return nil }
		// do not collect dirs that name is started by _
		if strings.HasPrefix(filepath.Base(path), "_") {
			return nil
		}

		// do not collect nested dirs
		hasPrefix := func() bool {
			for _, v := range targets {
				if strings.HasPrefix(filepath.Clean(path), filepath.Clean(string(v))) {
					return true
				}
			}
			return false
		}()
		if hasPrefix {
			return nil
		}

		targets = append(targets, targetPath(path))
		return nil;

	}); err != nil {
		return nil, err
	}

	return targets, nil
}

type ProcConfigSets map[string]*ProcConfigSet		// map[name]

func MakeProcConfigs(genBaseDir string) ProcConfigSets {
	paths, err := globProcConfigPaths(genBaseDir)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(paths)

	procConfigSets := make(ProcConfigSets)
	for _, v := range paths {
		tc := makeProcConfigSet(v)
		procConfigSets[tc.Name] = tc
	}

	return procConfigSets
}
