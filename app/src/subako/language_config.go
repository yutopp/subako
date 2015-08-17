package subako

import (
	"log"
	"os"

	"encoding/json"

	"path"
	"path/filepath"
	"fmt"
	"strings"

	"io/ioutil"
)


type LanguageName		string
type LanguageVersion	string


// Unit
// TODO:
// if version contains charactors sush as '/', make them error
type LangConfig struct {
	name				LanguageName
	version				LanguageVersion
}


// Set
type LangConfigSet struct {
	Name				LanguageName			`json:"name"`
	Versions			[]LanguageVersion		`json:"versions"`
	Type				string					`json:"type"`

	Configs				map[LanguageVersion]*LangConfig
	ProfileTemplate		*ProfileTemplate
	ProfilePatches		[]*ProfilePatch
}


func makeLangConfigSet(baseDir targetPath) (*LangConfigSet, error) {
	// load language config
	configPath := path.Join(string(baseDir), "config.json")
	log.Println("lang config path", configPath);

	file, err := ioutil.ReadFile(configPath)
    if err != nil {
        return nil, err
    }

	configSet := &LangConfigSet{
		Configs: make(map[LanguageVersion]*LangConfig),
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


	// read config
	for _, version := range configSet.Versions {
		config := &LangConfig{
			name: configSet.Name,
			version: version,
		}

		configSet.Configs[version] = config
	}


	// templates
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

	log.Printf("Results: %v\n", *configSet)

	// update
	configSet.ProfileTemplate = profileTemplate
	configSet.ProfilePatches = patches

	return configSet, nil
}
