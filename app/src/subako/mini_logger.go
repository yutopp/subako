package subako

import (
	"github.com/jinzhu/gorm"
)


type IMiniLogger interface {
	Failed(title, body string)
	Succeeded(body string)
	GetLatest(num int) []MiniLog
}


type MiniLog struct {
	gorm.Model
	Title		string
	Body		string
	Status		int
}

type MiniLogger struct {
	Db		gorm.DB
}

func MakeMiniLogger(db gorm.DB) (*MiniLogger, error) {
	db.AutoMigrate(&MiniLog{})

	return &MiniLogger{
		Db: db,
	}, nil
}

func (l *MiniLogger) Failed(title, body string) {
	l.Db.Debug().Create(&MiniLog{
		Title: title,
		Body: body,
		Status: 1,
	})
}

func (l *MiniLogger) Succeeded(title string) {
	l.Db.Debug().Create(&MiniLog{
		Title: title,
		Body: "",
		Status: 0,
	})
}

func (l *MiniLogger) GetLatest(num int) []MiniLog {
	logs := []MiniLog{}
	l.Db.Debug().Order("created_at desc").Limit(num).Find(&logs)

	return logs
}
