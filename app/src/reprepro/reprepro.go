package reprepro

const ExecName = "reprepro"

type Reprepro struct {
	RepositoryPath	string
	CodeName		string
}

func (r *Reprepro) MakeAddCommand(packagePath string) []string {
	args := r.makeIncludePart(packagePath)
	return append([]string{"reprepro", "-b", r.RepositoryPath}, args...)
}

func (r *Reprepro) MakeLsCommand(packagePath string) []string {
	args := r.makeListPart()
	return append([]string{"reprepro", "-b", r.RepositoryPath}, args...)
}

func (r *Reprepro) MakeRemoveCommand(packageName string) []string {
	args := r.makeRemovePart(packageName)
	return append([]string{"reprepro", "-b", r.RepositoryPath}, args...)
	// "deleteunreferenced"
}


func (r *Reprepro) makeIncludePart(packagePath string) []string {
	return []string{"includedeb", r.CodeName, packagePath}
}

func (r *Reprepro) makeListPart() []string {
	return []string{"list", r.CodeName}
}

func (r *Reprepro) makeRemovePart(packageName string) []string {
	return []string{"remove", r.CodeName, packageName}
}
