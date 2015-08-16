package subako

import (
	"fmt"
	"log"
	"os/exec"
	"bytes"
)


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
