package subako

import (
	"github.com/jinzhu/gorm"
)


type Crontab struct {
	Hour	int
	Minute	int
}

type DailyTask struct {
	gorm.Model
	ProcName	string
	Version		string
}

type DailyTasksContext struct {
	Db		gorm.DB
	Point	Crontab
	Logger	IMiniLogger
}

func MakeDailyTasksContext(db gorm.DB, point Crontab) (*DailyTasksContext, error) {
	db.AutoMigrate(&DailyTask{})

	return &DailyTasksContext{
		Db: db,
		Point: point,
	}, nil
}

func (ctx *DailyTasksContext) GetDailyTasks() []DailyTask {
	tasks := []DailyTask{}
	ctx.Db.Debug().Find(&tasks)

	return tasks
}

func (ctx *DailyTasksContext) Append(task *DailyTask) error {
	// TODO: error handling
	ctx.Db.Debug().Create(task)

	return nil
}

func (ctx *DailyTasksContext) Update(id uint, task *DailyTask) error {
	// TODO: error handling
	task.ID = id
	ctx.Db.Debug().Save(task)

	return nil
}

func (ctx *DailyTasksContext) Delete(id uint) error {
	// TODO: error handling
	task := &DailyTask{}
	task.ID = id

	ctx.Db.Debug().Delete(task)

	return nil
}
