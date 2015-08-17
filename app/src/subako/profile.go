package subako

import (
	"log"
	"sync"
	"fmt"
	"reflect"
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
	Exec				*ExecProfile	`json:"exec"`
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
	if p.Exec != nil {
		log.Println(" Exec")
		p.Exec.Log()
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

type propVerMap map[LanguageVersion][]GenericTemplate
type propMap map[LanguageName]propVerMap

func (p propMap) has(name LanguageName, version LanguageVersion) bool {
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
	aps		*AvailablePackages,
	pc		ProcConfigMap,
) error {
	targetProfileTemplates := make(propMap)

	// collect normal profile from available packages
	if err := aps.Walk(func(
		pkgName		PackageName,
		pkgVersion	PackageVersion,
		depName		PackageName,
		depVersion	PackageVersion,
		ap			*AvailablePackage,
	) error {
		pkgBuildConfigSet, ok := pc[pkgName]
		if !ok {
			return fmt.Errorf("there are no langname(%s)", pkgName)
		}

		for langName, langConfigSet := range pkgBuildConfigSet.LangConfigs {
			if _, ok := targetProfileTemplates[langName]; !ok {
				targetProfileTemplates[langName] = make(propVerMap)
			}

			langVersion := LanguageVersion(pkgVersion)
			if _, ok := langConfigSet.Configs[langVersion]; !ok {
				continue
			}

			log.Printf("Template: package(%s, %s) lang(%s, %s)", pkgName, pkgVersion, langName, langVersion)
			if langConfigSet.ProfileTemplate == nil {
				log.Printf("NOTE: Template: package(%s, %s) lang(%s, %s) is nil", pkgName, pkgVersion, langName, langVersion)
				continue
			}

			if targetProfileTemplates.has(langName, langVersion) {
				return fmt.Errorf("Profile: lang(%s, %s) is already registered", langName, langVersion)
			}

			// append
			targetProfileTemplates[langName][langVersion] = []GenericTemplate{
				GenericTemplate{
					Template: langConfigSet.ProfileTemplate,
					Ref: *ap,
				},
			}
		}

		return nil

	}); err != nil {
		return err
	}


	// patches
	// collect normal profile from available packages
	if err := aps.Walk(func(
		pkgName		PackageName,
		pkgVersion	PackageVersion,
		depName		PackageName,
		depVersion	PackageVersion,
		ap			*AvailablePackage,
	) error {
		pkgBuildConfigSet, ok := pc[pkgName]
		if !ok {
			return fmt.Errorf("there are no langname(%s)", pkgName)
		}

		for langName, langConfigSet := range pkgBuildConfigSet.LangConfigs {
			for _, patch := range langConfigSet.ProfilePatches {
				from := patch.From		// of langName / pkgName
				to := patch.To

				for _, fromVersion := range from.Versions {
					// if this package has this version, do action
					if !targetProfileTemplates.has(langName, fromVersion) {
						log.Printf("No Patch FROM %s / %s\n", string(pkgName), fromVersion)
						continue
					}

					for _, toVersion := range to.Versions {
						if !targetProfileTemplates.has(to.Name, toVersion) {
							// there are no targets, ignore
							log.Printf("No Patch TO %s / %s\n", to.Name, toVersion)
							continue
						}

						log.Printf("Template Patch FROM (%s, %s) TO (%s, %s)", langName, fromVersion, to.Name, toVersion)
						if targetProfileTemplates[to.Name][toVersion] == nil {
							targetProfileTemplates[to.Name][toVersion] = []GenericTemplate{}
						}

						targetProfileTemplates[to.Name][toVersion] = append(
							targetProfileTemplates[to.Name][toVersion],
							GenericTemplate{
								Template: patch,
								Ref: *ap,
							},
						)
					}
				}
			}
		}

		return nil

	}); err != nil {
		return err
	}


	// GENERATE
	profiles := make([]Profile, 0, 20)
	for name, vers := range targetProfileTemplates {
		for version, gens := range vers {
			prof := Profile{
				Name: string(name),
				Version: string(version),
			}

			log.Printf("Generate (%s, %s) / num = %d", name, version, len(gens))
			if len(gens) == 1 {
				if reflect.ValueOf(gens[0].Template).IsNil() {
					log.Printf("SKIPPED")
					continue
				}
			}

			for _, gen := range gens {
				if reflect.ValueOf(gen.Template).IsNil() {
					continue
				}

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
