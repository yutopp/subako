package subako

import (
	"log"
	"os"
	"time"
	"path/filepath"
	"fmt"
	"sync"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"

	"github.com/robfig/cron"
)

type SubakoConfig struct {
	ProcConfigSetsConf		*ProcConfigSetsConfig
	AvailablePackagesPath	string
	AptRepositoryBaseDir	string
	VirtualUsrDir			string
	TmpBaseDir				string
	PackagesDir				string
	RunningTasksPath		string
	ProfilesHolderPath		string
	DataBasePath			string
	NotificationConf		*NotificationConfig
	CronData				Crontab
	LogDir					string
}


type QueueTask struct {
	proc	ProcConfig
}


type SubakoContext struct {
	AptRepoCtx			*AptRepositoryContext
	BuilderCtx			*BuilderContext
	ProcConfigSetsCtx	*ProcConfigSetsContext
	AvailablePackages	*AvailablePackages
	RunningTasks		*RunningTasks
	Profiles			*ProfilesHolder
	Webhooks			*WebhookContext
	NotificationCtx		*NotificationContext
	DailyTasks			*DailyTasksContext
	LogDir				string

	queueCh				chan QueueTask
	QueueHelper			[]QueueTask

	m					sync.Mutex
}

func MakeSubakoContext(config *SubakoConfig) (*SubakoContext, error) {
	// Apt
	aptRepo, err := MakeAptRepositoryContext(config.AptRepositoryBaseDir)
	if err != nil {
		panic("error")
	}

	// Builder
	builderCtx, err := MakeBuilderContext(
		config.VirtualUsrDir,
		config.TmpBaseDir,
		config.PackagesDir,
	)
	if err != nil {
		panic("error")
	}

	// Config Sets
	procConfigSetsCtx, err := MakeProcConfigSetsContext(config.ProcConfigSetsConf)
	if err != nil {
		panic(err)
	}

	// Available Packages
	availablePackages, err := LoadAvailablePackages(config.AvailablePackagesPath)
	if err != nil {
		panic(err)
	}

	// running tasks
	runningTasks, err := LoadRunningTasks(config.RunningTasksPath)
	if err != nil {
		panic(err)
	}

	// profiles holder
	profiles, err := LoadProfilesHolder(config.ProfilesHolderPath)
	if err != nil {
		panic(err)
	}

	// load database
	db, err := gorm.Open("sqlite3", config.DataBasePath)
	if err != nil {
		panic(err)
	}

	// webhook holder
	webhooks, err := MakeWebhooksContext(db)
	if err != nil {
		panic(err)
	}

	// notification
	notificationCtx, err := MakeNotificationContext(config.NotificationConf)
	if err != nil {
		panic(err)
	}

	// cron
	dailyTasks, err := MakeDailyTasksContext(db, config.CronData)
	if err != nil {
		panic(err)
	}

	// make context
	ctx := &SubakoContext{
		AptRepoCtx: aptRepo,
		BuilderCtx: builderCtx,
		ProcConfigSetsCtx: procConfigSetsCtx,
		AvailablePackages: availablePackages,
		RunningTasks: runningTasks,
		Profiles: profiles,
		Webhooks: webhooks,
		NotificationCtx: notificationCtx,
		DailyTasks: dailyTasks,
		LogDir: config.LogDir,

		queueCh: make(chan QueueTask, 100),
		QueueHelper: make([]QueueTask, 0, 100),
	}

	go ctx.execQueuedTask()

	// cron
	cronText := fmt.Sprintf("00 %02d %02d * * *", config.CronData.Minute, config.CronData.Hour)
	c := cron.New()
	// sec, min, hour / every
	c.AddFunc(cronText, func() { ctx.queueDailyTask() })
	// c.AddFunc("10 * * * * *", func() { ctx.queueDailyTask() })	// test
	c.Start()

	return ctx, nil
}


func (ctx *SubakoContext) BuildAsync(
	taskConfig			*ProcConfig,
) *RunningTask {
	task := ctx.RunningTasks.createTaskHolder()
	go ctx.Build(taskConfig, task)

	return task
}

func (ctx *SubakoContext) Build(
	taskConfig			*ProcConfig,
	task				*RunningTask,
) *RunningTask {
	if task == nil {
		task = ctx.RunningTasks.createTaskHolder()
	}

	task.IsActive = true
	defer func() { task.IsActive = false }()
	task.Status = TaskRunning

	logName := fmt.Sprintf("%s-%s-%s", taskConfig.name, taskConfig.version, time.Now())
	task.LogName = logName

	logFileName := fmt.Sprintf("log-%s.log", logName)
	logFilePath := filepath.Clean(filepath.Join(ctx.LogDir, logFileName))
	w, err := os.OpenFile(logFilePath, os.O_CREATE | os.O_RDWR, 0644)
	if err != nil {
		log.Printf("Failed to openfile %s", logFilePath)
		task.ErrorText = "failed to open log reciever"
		task.Status = TaskFailed
		return task
	}
	defer w.Close()
	task.LogFilePath = logFilePath

	result, err := ctx.BuilderCtx.build(taskConfig, ctx.ProcConfigSetsCtx.BaseDir, w)
	if err != nil {
		log.Printf("Failed to build / %v", err)
		task.ErrorText = err.Error()
		task.Status = TaskFailed

		w.Write([]byte(fmt.Sprintf("Error occured => %s\n", err)))

		return task
	}

	// update available packages
	if err := ctx.AvailablePackages.Update(taskConfig.name, &AvailablePackage{
		Version: taskConfig.version,
		PackageFileName: result.PkgFileName,
		PackageName: result.PkgName,
		PackageVersion: result.PkgVersion,
		DisplayVersion: result.DisplayVersion,
		InstallBase: "/usr/local/torigoya",		// TODO: fix
	}); err != nil {
		task.ErrorText = err.Error()
		task.Status = TaskFailed

		return task
	}

	// update profiles
	if err := ctx.UpdateProfiles(); err != nil {
		task.ErrorText = err.Error()
		task.Status = TaskFailed

		return task
	}

	// update repository
	debPath := filepath.Join(ctx.BuilderCtx.packagesDir, result.PkgFileName)
	if err := ctx.AptRepoCtx.AddPackage(debPath); err != nil {
		task.ErrorText = err.Error()
		task.Status = TaskFailed

		return task
	}

	// TODO: fix...
	// remove a source deb file to save free storage
	if err := os.Remove(debPath); err != nil {
		task.ErrorText = "failed to remove source deb"
		task.Status = TaskFailed

		return task
	}

	// notify
	if ctx.NotificationCtx != nil {
		if err := ctx.NotificationCtx.PostUpdate(map[string]string{
		}); err != nil {
			task.ErrorText = err.Error()
			task.Status = TaskWarning

			return task
		}
	}

	task.Status = TaskSucceeded

	return task
}


func (ctx *SubakoContext) Queue(
	procConfig			*ProcConfig,
) error {
	ctx.m.Lock()
	defer ctx.m.Unlock()

	task := QueueTask{
		proc: *procConfig,
	}

	ctx.QueueHelper = append(ctx.QueueHelper, task)
	ctx.queueCh <- task

	return nil
}


func (ctx *SubakoContext) Save(
) error {
	if err := ctx.AvailablePackages.Save(); err != nil {
		return err
	}

	if err := ctx.RunningTasks.Save(); err != nil {
		return err
	}

	if err := ctx.Profiles.Save(); err != nil {
		return err
	}

	return nil
}


// running on goroutine
func (ctx *SubakoContext) execQueuedTask() {
	for q := range ctx.queueCh {
		ctx.m.Lock()
		if len(ctx.QueueHelper) > 0 {
			ctx.QueueHelper = ctx.QueueHelper[1:]
		}
		ctx.m.Unlock()

		ctx.Build(&q.proc, nil)
	}
}

func (ctx *SubakoContext) queueDailyTask() {
	log.Println("QueueDailyTask is called")

	tasks := ctx.DailyTasks.GetDailyTasks()
	for _, task := range tasks {
		proc, err := ctx.ProcConfigSetsCtx.Find(task.ProcName, task.Version)
		if err != nil {
			log.Printf("Failed to find the task :: name: %s / version: %s", task.ProcName, task.Version)
			continue
		}

		log.Printf("QueueDailyTask queue :: name: %s / version: %s", task.ProcName, task.Version)
		if err := ctx.Queue(proc); err != nil {
			log.Println("Failed to queue the task")
			continue
		}
	}
}


func (ctx *SubakoContext) UpdateProfiles() error {
	return ctx.Profiles.GenerateProcProfiles(
		ctx.AvailablePackages,
		ctx.ProcConfigSetsCtx.Map,
	)
}
