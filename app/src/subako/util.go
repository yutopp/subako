package subako

import (
	"log"
	"os"
)


// exists returns whether the given file or directory exists or not
func Exists(path string) bool {
    _, err := os.Stat(path)
    if err == nil { return true }
    if os.IsNotExist(err) { return false }
    return true
}

func exactFilePath(path string) (string, error) {
	if !Exists(path) {
		log.Println("create dir: ", path);
		if err := os.Mkdir(path, 0755); err != nil {
			return "", err
		}
	}

	return path, nil
}


func maxI(a, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

func minI(a, b int) int {
	if a > b {
		return b
	} else {
		return a
	}
}
