package subako

import (
	"log"
	"os"
	"os/exec"
	"reprepro"
)


type AptRepositoryContext struct {
	AptRepositoryBaseDir	string
	reprepro				*reprepro.Reprepro
}

func MakeAptRepositoryContext(
	aptRepositoryBaseDir	string,
) (*AptRepositoryContext, error) {
	// existence
	if !Exists(aptRepositoryBaseDir) {
		if err := os.Mkdir(aptRepositoryBaseDir, 0755); err != nil {
			return nil, err
		}
	}

	return &AptRepositoryContext{
		AptRepositoryBaseDir: aptRepositoryBaseDir,
		reprepro: &reprepro.Reprepro{
			RepositoryPath: aptRepositoryBaseDir,
			CodeName: "trusty",		// for 14.04 LTS
		},
	}, nil
}

func (ctx *AptRepositoryContext) AddPackage(debPath string) error {
	args := ctx.reprepro.MakeAddCommand(debPath)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil { return err }

	return nil
}


func (ctx *AptRepositoryContext) RemovePackage(pkgName string) error {
	log.Printf("REMOVE: repoPath(%s)", ctx.reprepro.RepositoryPath)

	args := ctx.reprepro.MakeRemoveCommand(pkgName)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil { return err }

	return nil
}
