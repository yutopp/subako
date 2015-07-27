package subako

import (
	"errors"
	"log"
	"sync"
)


type ExecProfile ExecProfileTemplate
type Profile struct {
	Name				string
	Version				string
	DisplayVersion		string
	IsBuildRequired		bool
	IsLinkIndependent	bool

	Compile				*ExecProfile
	Link				*ExecProfile
	Run					*ExecProfile
}


type GenericTemplate struct {
	Template	IProfileTemplate
	Ref			AvailablePackage
}

func (h *GenericTemplate) Update(p *Profile) {
	h.Template.Generate(p, &h.Ref)
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
			return errors.New("")		// TODO: fix
		}
		targetProfileTemplates[name] = make(propVerMap)

		for version, apPkg := range apPkgs {
			log.Printf("Gen %s %s\n", name, version)
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

			log.Printf("PATCH %v\n", patch)
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

			log.Printf("GENGEN %s %s\n", name, version)
			for _, gen := range gens {
				gen.Update(&prof)
			}

			log.Println("GENERATED PROF", prof)
			profiles = append(profiles, prof)
		}
	}

	log.Println("targetProfileTemplates", targetProfileTemplates)

	// Update
	ph.Profiles = profiles

	return nil
}
