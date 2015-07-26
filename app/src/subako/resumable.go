package subako

import (
	"io/ioutil"
	"encoding/json"
)


type Resumable interface {
	SetFilePath(string)
	GetFilePath() string
}

func LoadStructure(path string, r Resumable) error {
	r.SetFilePath(path)

	if !exists(path) {
		return nil
	}

	buffer, err := ioutil.ReadFile(path)
    if err != nil {
		return err
    }

	if err := json.Unmarshal(buffer, &r); err != nil {
		return err
	}

	return nil
}

func SaveStructure(r Resumable) error {
	buffer, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(r.GetFilePath(), buffer, 0644); err != nil {
		return err
	}

	return nil
}


type HasFilePath struct {
	FilePath	string		`json:"-"`	// ignore
}

func (f *HasFilePath) SetFilePath(path string) {
	f.FilePath = path
}

func (f *HasFilePath) GetFilePath() string {
	return f.FilePath
}
