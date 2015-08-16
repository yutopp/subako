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
	PackagePrefix			string
	InstallBasePrefix		string

	RunningTasksPath		string
	ProfilesHolderPath		string
	DataBasePath			string
	NotificationConf		*NotificationConfig
	CronData				Crontab
	LogDir					string
}


type QueueTask struct {
	Proc	IPackageBuildConfig
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
	Logger				IMiniLogger		// mini logger

	queueCh				chan QueueTask
	QueueHelper			[]QueueTask

	m					sync.Mutex
}

func MakeSubakoContext(config *SubakoConfig) (*SubakoContext, error) {
	// load database
	db, err := gorm.Open("sqlite3", config.DataBasePath)
	if err != nil {
		panic(err)
	}

	// logger
	miniLogger, err := MakeMiniLogger(db)
	if err != nil {
		panic("error")
	}

	// Apt
	aptRepo, err := MakeAptRepositoryContext(config.AptRepositoryBaseDir)
	if err != nil {
		panic("error")
	}

	// Builder
	builderCtx, err := MakeBuilderContext(&BuilderConfig{
		virtualUsrDir: config.VirtualUsrDir,
		tmpBaseDir: config.TmpBaseDir,
		packagesDir: config.PackagesDir,
		packagePrefix: config.PackagePrefix,
		installBasePrefix: config.InstallBasePrefix,
	})
	if err != nil {
		panic(err)
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
		Logger: miniLogger,

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
	taskConfig			IPackageBuildConfig,
) *RunningTask {
	task := ctx.RunningTasks.createTaskHolder()
	go ctx.Build(taskConfig, task)

	return task
}

func (ctx *SubakoContext) Build(
	taskConfig			IPackageBuildConfig,
	task				*RunningTask,
) *RunningTask {
	if task == nil {
		task = ctx.RunningTasks.createTaskHolder()
	}

	task.Status = TaskRunning

	logName := fmt.Sprintf("%s-%s-%s", taskConfig.GetName(), taskConfig.GetVersion(), time.Now().Format("2006-01-02 15:04:05 MST"))
	task.LogName = logName

	logFileName := fmt.Sprintf("log-%s.log", logName)
	logFilePath := filepath.Clean(filepath.Join(ctx.LogDir, logFileName))
	w, err := os.OpenFile(logFilePath, os.O_CREATE | os.O_RDWR, 0644)
	if err != nil {
		log.Printf("Failed to openfile %s", logFilePath)
		task.Failed("failed to open log reciever")

		return task
	}
	defer w.Close()
	task.LogFilePath = logFilePath

	ch := make(chan IntermediateContainerInfo)
	go func() {
		ici := <-ch
		log.Printf("Got Container information: %v", ici)
		task.ContainerID = &ici.ContainerID
		task.KillContainer = &ici.KillContainerFunc
	}()
	result, err := ctx.BuilderCtx.build(taskConfig, ctx.ProcConfigSetsCtx.BaseDir, w, ch)
	if err != nil {
		log.Printf("Failed to build / %v", err)
		task.Failed(err.Error())

		w.Write([]byte(fmt.Sprintf("Error occured => %s\n", err)))

		ctx.Logger.Failed(fmt.Sprintf("Failed to build: %s / %s", taskConfig.GetName(), taskConfig.GetVersion()), task.ErrorText)

		return task
	}

	// update available packages
	if err := ctx.AvailablePackages.Update(&AvailablePackage{
		Name: taskConfig.GetName(),
		Version: taskConfig.GetVersion(),
		DisplayVersion: result.DisplayVersion,

		GeneratedPackageFileName: result.PkgFileName,
		GeneratedPackageName: result.PkgName,
		GeneratedPackageVersion: result.PkgVersion,

		InstallBase: result.hostInstallBase,
		InstallPrefix: result.hostInstallPrefix,

		DepName: taskConfig.GetDepName(),
		DepVersion: taskConfig.GetDepVersion(),
	}); err != nil {
		task.Failed(err.Error())
		ctx.Logger.Failed(fmt.Sprintf("Failed to update packages: %s / %s", taskConfig.GetName(), taskConfig.GetVersion()), task.ErrorText)

		return task
	}

	// update repository
	debPath := filepath.Join(ctx.BuilderCtx.packagesDir, result.PkgFileName)
	if err := ctx.AptRepoCtx.AddPackage(debPath); err != nil {
		task.Failed(err.Error())

		ctx.Logger.Failed(fmt.Sprintf("Failed to update repo: %s / %s", taskConfig.GetName(), taskConfig.GetVersion()), task.ErrorText)

		return task
	}

	// TODO: fix...
	// remove a source deb file to save free storage
	if err := os.Remove(debPath); err != nil {
		task.Failed("failed to remove source deb")

		ctx.Logger.Failed(fmt.Sprintf("Failed to remove deb: %s / %s", taskConfig.GetName(), taskConfig.GetVersion()), task.ErrorText)

		return task
	}

	// notify
	if ctx.NotificationCtx != nil {
		if err := ctx.NotificationCtx.PostUpdate(map[string]string{
			"type": "package_update",
			"name": string(taskConfig.GetName()),
			"version": string(taskConfig.GetVersion()),
			"display_version": result.DisplayVersion,
			"unix_time": fmt.Sprintf("%v", time.Now().Unix()),
		}); err != nil {
			task.Warning(err.Error())

			ctx.Logger.Failed(fmt.Sprintf("Failed to notification: %s / %s", taskConfig.GetName(), taskConfig.GetVersion()), task.ErrorText)

			return task
		}
	}

	// update profiles
	if err := ctx.UpdateProfilesWithNotification(); err != nil {
		task.Warning(err.Error())

		return task
	}
	task.Status = TaskSucceeded

	ctx.Logger.Succeeded(fmt.Sprintf("Build: %s / %s [%v]", taskConfig.GetName(), taskConfig.GetVersion(), result.duration))

	return task
}


func (ctx *SubakoContext) Queue(
	procConfig			IPackageBuildConfig,
) error {
	ctx.m.Lock()
	defer ctx.m.Unlock()

	task := QueueTask{
		Proc: procConfig,
	}

	ctx.QueueHelper = append(ctx.QueueHelper, task)
	ctx.queueCh <- task

	ctx.Logger.Succeeded(fmt.Sprintf("Queue the task: %s / %s", procConfig.GetName(), procConfig.GetVersion()))

	return nil
}


func (ctx *SubakoContext) Save() error {
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

		ctx.Build(q.Proc, nil)
	}
}

func (ctx *SubakoContext) queueDailyTask() {
	log.Println("QueueDailyTask is called")
	ctx.Logger.Succeeded("QueueDailyTask starts")

	tasks := ctx.DailyTasks.GetDailyTasks()
	for _, task := range tasks {
		proc, err := ctx.ProcConfigSetsCtx.Find(task.ProcName, task.Version)
		if err != nil {
			msg := fmt.Sprintf("Failed to find the task :: name: %s / version: %s", task.ProcName, task.Version)
			log.Println(msg)
			ctx.Logger.Failed("DailyTask", msg)

			continue
		}

		log.Printf("QueueDailyTask queue :: name: %s / version: %s", task.ProcName, task.Version)
		if err := ctx.Queue(proc); err != nil {
			msg := "Failed to queue the task"
			log.Println(msg)
			ctx.Logger.Failed("DailyTask", msg)

			continue
		}
	}

	ctx.Logger.Succeeded("QueueDailyTask finished")
}


func (ctx *SubakoContext) UpdateProfiles() error {
	if err := ctx.Profiles.GenerateProcProfiles(
		ctx.AvailablePackages,
		ctx.ProcConfigSetsCtx.Map,
	); err != nil {
		ctx.Logger.Failed("UpdateProfiles", err.Error())
		return err
	}

	ctx.Logger.Succeeded("UpdateProfiles")
	return nil
}


func (ctx *SubakoContext) UpdateProfilesWithNotification() error {
	if err := ctx.UpdateProfiles(); err != nil {
		return err
	}

	if ctx.NotificationCtx != nil {
		if err := ctx.NotificationCtx.PostUpdate(map[string]string{
			"type": "profile_update",
		}); err != nil {
			ctx.Logger.Failed("UpdateProfilesWithNotification", err.Error())
			return err
		}
	}

	ctx.Logger.Succeeded("UpdateProfilesWithNotification")
	return nil
}


func (ctx *SubakoContext) RefreshProfileConfigs() error {
	if err := ctx.ProcConfigSetsCtx.Update(); err != nil {
		ctx.Logger.Failed("RefreshProfileConfigs", err.Error())
		return err
	}

	if err := ctx.UpdateProfilesWithNotification(); err != nil {
		return err
	}

	ctx.Logger.Succeeded("RefreshProfileConfigs")
	return nil
}


func (ctx *SubakoContext) RemovePackage(name, version string) error {
	return ctx.RemovePackageDep(name, version, "", "")
}


func (ctx *SubakoContext) RemovePackageDep(name, version, depName, DepVersion string) error {
	if err := ctx.AvailablePackages.Remove(name, version, depName, DepVersion); err != nil {
		ctx.Logger.Failed("RemovePackage", err.Error())
		return err
	}

	ctx.Logger.Succeeded(fmt.Sprintf("RemovePackage: %s / %s", name, version))

	return ctx.UpdateProfilesWithNotification()
}
