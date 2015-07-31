package subako

import (
	"errors"
	"log"
	"sync"
	"fmt"
)


type ExecProfile ExecProfileTemplate
type Profile struct {
	Name				string			`json:"name"`
	Version				string			`json:"version"`
	DisplayVersion		string			`json:"display_version"`
	IsBuildRequired		bool			`json:"is_build_required"`
	IsLinkIndependent	bool			`json:"is_link_independent"`

	Compile				*ExecProfile	`json:"compile"`
	Link				*ExecProfile	`json:"link"`
	Run					*ExecProfile	`json:"run"`
}

func (p *Profile) Log() {
	log.Printf(" Name = %s\n", p.Name)
	log.Printf(" Version = %s\n", p.Version)
	log.Printf(" DisplayVersion = %s\n", p.DisplayVersion)
	log.Printf(" IsBuildRequired = %s\n", p.IsBuildRequired)
	log.Printf(" IsLinkIndependent = %s\n", p.IsLinkIndependent)
	if p.Compile != nil {
		log.Println(" Compile")
		p.Compile.Log()
	}
	if p.Link != nil {
		log.Println(" Link")
		p.Link.Log()
	}
	if p.Run != nil {
		log.Println(" Run")
		p.Run.Log()
	}
}

func (e *ExecProfile) Log() {
	log.Printf("  - Extension = %s\n", e.Extension)
	log.Printf("  - Commands = %s\n", e.Commands)
	log.Printf("  - Envs = %s\n", e.Envs)
	log.Printf("  - FixedCommands = %s\n", e.FixedCommands)
	log.Printf("  - SelectableOptions = %s\n", e.SelectableOptions)
	log.Printf("  - CpuLimit = %s\n", e.CpuLimit)
	log.Printf("  - MemoryLimit = %s\n", e.MemoryLimit)
}


type GenericTemplate struct {
	Template	IProfileTemplate
	Ref			AvailablePackage
}

func (h *GenericTemplate) Update(p *Profile) error {
	return h.Template.Generate(p, &h.Ref)
}

type propVerMap map[string][]GenericTemplate	// map[version]IProfileTemplate
type propMap map[string]propVerMap				// map[name]

func (p propMap) has(name, version string) bool {
	if _, ok := p[name]; !ok {
		return false
	}
	if _, ok := p[name][version]; !ok {
		return false
	}

	return true
}

type ProfilesHolder struct {
	Profiles	[]Profile

	m			sync.Mutex	`json:"-"`	// ignore when saving
	HasFilePath
}

func LoadProfilesHolder(path string) (*ProfilesHolder, error) {
	var ph ProfilesHolder
	if err := LoadStructure(path, &ph); err != nil {
		return nil, err
	}

	return &ph, nil
}

func (ph *ProfilesHolder) Save() error {
	ph.m.Lock()
	defer ph.m.Unlock()

	return SaveStructure(ph)
}

func (ph *ProfilesHolder) GenerateProcProfiles(
	ap	*AvailablePackages,
	pc	ProcConfigMap,
) error {
	targetProfileTemplates := make(propMap)

	// collect normal profile from available packages
	for name, apPkgs := range ap.Packages {
		procConfig, ok := pc[name]
		if !ok {
			return fmt.Errorf("")		// TODO: fix
		}
		targetProfileTemplates[name] = make(propVerMap)

		for version, apPkg := range apPkgs {
			log.Printf("Template %s %s\n", name, version)
			targetProfileTemplates[name][version] = []GenericTemplate{
				GenericTemplate{
					Template: procConfig.ProfileTemplate,
					Ref: apPkg,
				},
			}
		}
	}

	// patches
	for name, _ := range ap.Packages {
		procConfig, ok := pc[name]
		if !ok {
			return errors.New("")		// TODO: fix
		}
		for _, patch := range procConfig.ProfilePatches {
			from := patch.From
			to := patch.To

			for _, fromVersion := range from.Versions {
				// if this package has this version, do action
				if !targetProfileTemplates.has(name, fromVersion) {
					continue
				}

				log.Printf("FROM %v\n", fromVersion)

				//
				for _, toVersion := range to.Versions {
					if !targetProfileTemplates.has(to.Name, toVersion) {
						// there are no targets, ignore
						continue
					}

					targetProfileTemplates[to.Name][toVersion] = append(
						targetProfileTemplates[to.Name][toVersion],
						GenericTemplate{
							Template: patch,
							Ref: ap.Packages[name][fromVersion],
						},
					)
					log.Printf("TO %v\n", toVersion)
				}
			}

			log.Printf("Template Patch %v\n", patch)
		}
	}

	// GENERATE
	profiles := make([]Profile, 0)
	for name, vers := range targetProfileTemplates {
		for version, gens := range vers {
			prof := Profile{
				Name: name,
				Version: version,
			}

			log.Printf("Generate %s %s\n", name, version)
			for _, gen := range gens {
				if err := gen.Update(&prof); err != nil {
					log.Printf("Error: %v", err)
					return err
				}
			}

			prof.Log()
			log.Println("=====")

			profiles = append(profiles, prof)
		}
	}

	log.Println("targetProfileTemplates", targetProfileTemplates)

	// Update
	ph.Profiles = profiles

	return nil
}
