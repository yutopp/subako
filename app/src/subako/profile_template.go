package subako

import (
	"log"
	"errors"
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)


func readProfileTemplate(
	filePath string,
) (*ProfileTemplate, error) {
	buf, err := ioutil.ReadFile(filePath)
    if err != nil {
        return nil, err
    }

	var pt ProfileTemplate
	if err := yaml.Unmarshal(buf, &pt); err != nil {
		return nil, err
	}

	if err := validateExecProfileTemplate(pt.Compile); err != nil {
		return nil, fmt.Errorf("Compile section: %v", err)
	}
	if err := validateExecProfileTemplate(pt.Link); err != nil {
		return nil, fmt.Errorf("Link section: %v", err)
	}

	if pt.Exec == nil {
		return nil, errors.New("must contain 'exec' section")
	}
	if err := validateExecProfileTemplate(pt.Exec); err != nil {
		return nil, fmt.Errorf("Exec section: %v", err)
	}

	log.Println("ProfileTemplate => ", pt)

	return &pt, nil
}

func validateExecProfileTemplate(pt *ExecProfileTemplate) error {
	if pt == nil {
		return nil
	}

	if pt.Commands == nil || len(pt.Commands) == 0 {
		return errors.New("must contain 'commands' element")
	}

	if pt.CpuLimit == 0 {
		return errors.New("must contain 'cpu_limit' element and must not be 0")
	}

	if pt.MemoryLimit == 0 {
		return errors.New("must contain 'memory_limit' element and must not be 0")
	}

	return nil
}


func readProfilePatch(
	filePath string,
) (*ProfilePatch, error) {
	log.Printf("patch filepath: %s",filePath)

	buf, err := ioutil.ReadFile(filePath)
    if err != nil {
        return nil, err
    }

	var pt ProfilePatch
	if err := yaml.Unmarshal(buf, &pt); err != nil {
		return nil, err
	}

	log.Println("ProfilePatch => ", pt)

	return &pt, nil
}


type IProfileTemplate interface {
	Generate(*Profile, *AvailablePackage) error
}


// ==
type ProfileTemplate struct {
	DisplayVersion		string					`yaml:"display_version"`
	IsBuildRequired		bool					`yaml:"is_build_required"`
	IsLinkIndependent	bool					`yaml:"is_link_independent"`

	Compile				*ExecProfileTemplate	`yaml:"compile,omitempty"`
	Link				*ExecProfileTemplate	`yaml:"link"`
	Exec				*ExecProfileTemplate	`yaml:"exec"`
}

type ExecProfileTemplate struct {
	Extension			string					`yaml:"extension,omitempty" json:"extension"`
	Commands			[]string				`yaml:"commands" json:"commands"`
	Envs				map[string]string		`yaml:"envs,omitempty" json:"envs"`
	FixedCommands		[][]string				`yaml:"fixed_commands,omitempty" json:"fixed_commands"`
	SelectableOptions	map[string][]string		`yaml:"selectable_options,omitempty" json:"selectable_options"`
	CpuLimit			uint64					`yaml:"cpu_limit" json:"cpu_limit"`
	MemoryLimit			uint64					`yaml:"memory_limit,omitempty" json:"memory_limit"`
}


func (template *ProfileTemplate) Generate(
	profile *Profile,
	ap *AvailablePackage,
) (err error) {
	log.Printf("GENERATE by %s %s\n", ap.PackageName, ap.Version)

	profile.DisplayVersion, err = ap.ReplaceString(template.DisplayVersion)
	if err != nil { return err }
	profile.IsBuildRequired = template.IsBuildRequired
	profile.IsLinkIndependent = template.IsLinkIndependent

	profile.Compile, err = setExecProfile(ap, template.Compile)
	if err != nil { return }
	profile.Link, err = setExecProfile(ap, template.Link)
	if err != nil { return }
	profile.Exec, err = setExecProfile(ap, template.Exec)
	if err != nil { return }

	return
}


// ==
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
		Exec				*ExecProfileTemplate
	}
}


func (patch *ProfilePatch) Generate(
	profile *Profile,
	ap *AvailablePackage,
) (err error) {
	log.Printf("GENERATE by %s %s\n", ap.PackageName, ap.Version)

	profile.Compile, err = appendExecProfile(ap, profile.Compile, patch.Append.Compile)
	if err != nil { return err }

	profile.Link, err = appendExecProfile(ap, profile.Link, patch.Append.Link)
	if err != nil { return }

	profile.Exec, err = appendExecProfile(ap, profile.Exec, patch.Append.Exec)
	if err != nil { return }

	return
}


// ==
func setExecProfile(
	ap *AvailablePackage,
	src *ExecProfileTemplate,
) (prof *ExecProfile, err error) {
	if src == nil { return nil, nil }

	prof = &ExecProfile{}

	prof.Extension, err = ap.ReplaceString(src.Extension)
	if err != nil { return }

	prof.Commands, err = transformStringArray(src.Commands, ap.ReplaceString)
	if err != nil { return }

	prof.Envs, err = transformStringMap(src.Envs, ap.ReplaceString)
	if err != nil { return }

	prof.FixedCommands, err = transformStringNestedArray(src.FixedCommands, ap.ReplaceString)
	if err != nil { return }

	prof.SelectableOptions, err = transformStringArrayMap(src.SelectableOptions, ap.ReplaceString)
	if err != nil { return }

	prof.CpuLimit = src.CpuLimit
	prof.MemoryLimit = src.MemoryLimit

	return
}

func appendExecProfile(
	ap *AvailablePackage,
	base *ExecProfile,
	src *ExecProfileTemplate,
) (prof *ExecProfile, err error) {
	if src == nil { return }

	var execProfile ExecProfile = *base
	prof = &execProfile

	commands, err := transformStringArray(src.Commands, ap.ReplaceString)
	if err != nil { return }
	prof.Commands = append(base.Commands, commands...)

	envs, err := transformStringMap(src.Envs, ap.ReplaceString)
	if err != nil { return }
	prof.Envs = appendStringMap(base.Envs, envs)

	fixedCommands, err := transformStringNestedArray(src.FixedCommands, ap.ReplaceString)
	if err != nil { return }
	prof.FixedCommands = append(base.FixedCommands, fixedCommands...)

	options, err := transformStringArrayMap(src.SelectableOptions, ap.ReplaceString)
	if err != nil { return }
	prof.SelectableOptions = appendStringArrayMap(base.SelectableOptions, options)

	return
}


// ==
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


type transF func(string) (string, error)

func transformStringArray(gen []string, f transF) ([]string, error) {
	result := make([]string, len(gen))
	var err error

	for i, v := range gen {
		result[i], err = f(v)
		if err != nil { return nil, err }
	}

	return result, nil
}

func transformStringNestedArray(gen [][]string, f transF) ([][]string, error) {
	result := make([][]string, len(gen))
	var err error

	for i, v := range gen {
		result[i], err = transformStringArray(v, f)
		if err != nil { return nil, err }
	}

	return result, nil
}

func transformStringMap(gen map[string]string, f transF) (map[string]string, error) {
	result := make(map[string]string)
	var err error

	for k, v := range gen {
		result[k], err = f(v)
		if err != nil { return nil, err }
	}

	return result, nil
}

func transformStringArrayMap(gen map[string][]string, f transF) (map[string][]string, error) {
	result := make(map[string][]string)
	var err error

	for k, v := range gen {
		result[k], err = transformStringArray(v, f)
		if err != nil { return nil, err }
	}

	return result, nil
}
