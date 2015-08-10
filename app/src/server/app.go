package main

import (
	"subako"

	"flag"
	"os"
	"log"
	"path/filepath"
	"net/http"
	"io/ioutil"

	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
	"github.com/goji/httpauth"

	"github.com/flosch/pongo2"
	"github.com/ActiveState/tail"

	"strconv"
	"encoding/json"
	"gopkg.in/yaml.v2"

	"time"
	"path"
	"fmt"

	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
)

var gSubakoCtx *subako.SubakoContext
const packageConfigsHookName = "__configs"

type userConfig struct {
	Server			struct {
		Port		int
	}
	Notification	struct {
		Url			string
		Secret		string
	}
	Cron			struct {
		Hour		int
		Minute		int
	}
	Auth			struct {
		User		string
		Password	string
	}
	Builder			*struct {
		PackagePrefix		string	`yaml:"package_prefix"`
		InstallBasePrefix	string	`yaml:"install_base_prefix"`
	}
	ConfigSets		struct {
		Remote		bool
		Path		string
		Repository	string
		RepoSecret	string	`yaml:"webhook_secret"`
	} `yaml:"config_sets"`
}

func main() {
	defer func() {
		log.Println("Exit main")
	}()

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// read user config
	buffer, err := ioutil.ReadFile(path.Join(cwd, "config.yml"),)
    if err != nil {
		log.Fatal(err)
    }
	var uConfig userConfig
	if err := yaml.Unmarshal(buffer, &uConfig); err != nil {
		log.Fatal(err)
	}

	//
	log.Printf("Port: %s", uConfig.Server.Port)
	log.Printf("Notification URL: %s", uConfig.Notification.Url)
	log.Printf("Cron Timing: %d:%d", uConfig.Cron.Hour, uConfig.Cron.Minute)
	log.Printf("ConfigSets IsRemote: %v", uConfig.ConfigSets.Remote)
	log.Printf("ConfigSets Path: %s", uConfig.ConfigSets.Path)
	if uConfig.Builder == nil {
		log.Fatal("builder section is not written in config")
	}
	log.Printf("PackagePrefix: %s", uConfig.Builder.PackagePrefix)
	log.Printf("InstallPrefix: %s", uConfig.Builder.InstallBasePrefix)
	if uConfig.ConfigSets.Remote {
		log.Printf("ConfigSets Repository: %s", uConfig.ConfigSets.Repository)
		log.Printf("ConfigSets RepoSecret: %s", uConfig.ConfigSets.RepoSecret)
	}

	// port
	flag.Set("bind", fmt.Sprintf(":%d", uConfig.Server.Port))

	// make storage dir
	storageDir := path.Join(cwd, "_storage")
	if !subako.Exists(storageDir) {
		if err := os.Mkdir(storageDir, 0755); err != nil {
			log.Fatal(err)
		}
	}

	// make config
	config := &subako.SubakoConfig{
		ProcConfigSetsConf: func() *subako.ProcConfigSetsConfig{
			path := func() string {
				if filepath.IsAbs(uConfig.ConfigSets.Path) {
					return uConfig.ConfigSets.Path
				} else {
					return path.Join(cwd, uConfig.ConfigSets.Path)
				}
			}()

			return &subako.ProcConfigSetsConfig{
				IsRemote: uConfig.ConfigSets.Remote,
				BaseDir: path,
				Repository: uConfig.ConfigSets.Repository,
			}
		}(),
		AvailablePackagesPath: path.Join(storageDir, "available_packages.json"),
		AptRepositoryBaseDir: path.Join(storageDir, "apt_repository"),

		VirtualUsrDir: path.Join(storageDir, "torigoya_usr"),
		TmpBaseDir: path.Join(storageDir, "temp"),
		PackagesDir: path.Join(storageDir, "packages"),
		PackagePrefix: uConfig.Builder.PackagePrefix,
		InstallBasePrefix: uConfig.Builder.InstallBasePrefix,

		RunningTasksPath: path.Join(storageDir, "running_tasks.json"),
		ProfilesHolderPath: path.Join(storageDir, "proc_profiles.json"),
		DataBasePath: path.Join(storageDir, "db.sqlite"),
		NotificationConf: &subako.NotificationConfig{
			TargetUrl: uConfig.Notification.Url,
			Secret: uConfig.Notification.Secret,
		},
		CronData: subako.Crontab {
			Hour: uConfig.Cron.Hour,
			Minute: uConfig.Cron.Minute,
		},
		LogDir: "/tmp",
	}

	subakoCtx, err := subako.MakeSubakoContext(config)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := subakoCtx.Save(); err != nil {
			panic(err)
		}
	}()

	// add hooks for '__configs' (*special usage*)
	func() {
		if packageConfigsHookName == "" { return }	// skip

		dummy_hook := &subako.Webhook{
			Target: packageConfigsHookName,
			ProcName: "*For config update*",
		}
		hook, err := subakoCtx.Webhooks.GetByTargetOrCreate(packageConfigsHookName, dummy_hook)
		if err != nil {
			panic(err)
		}
		log.Println(hook)

		hook.Secret = uConfig.ConfigSets.RepoSecret
		if err := subakoCtx.Webhooks.Update(hook.ID, hook); err != nil {
			panic(err)
		}
	}()

	//
	gSubakoCtx = subakoCtx

	//
	authOpts := httpauth.AuthOptions{
        Realm: "TorigoyaFactory",
        User: uConfig.Auth.User,
        Password: uConfig.Auth.Password,
    }
	reqAuthMux := web.New()
	reqAuthMux.Use(httpauth.BasicAuth(authOpts))

	//
	pongo2.DefaultSet.SetBaseDirectory("views")

	goji.Get("/assets/*", http.StripPrefix("/assets/", http.FileServer(http.Dir("./public"))))
	goji.Get("/apt/*", http.StripPrefix("/apt/", http.FileServer(http.Dir(subakoCtx.AptRepoCtx.AptRepositoryBaseDir))))

	goji.Get("/", index)

	reqAuthMux.Get("/live_status/:id", liveStatus)
	reqAuthMux.Get("/abort_task/:id", abortTask)
	goji.Get("/status/:id", status)

	reqAuthMux.Get("/build/:name/:version", build)
	reqAuthMux.Get("/queue/:name/:version", queue)

	goji.Get("/packages", showPackages)
	reqAuthMux.Get("/remove_package/:name/:version", removePackage)

	reqAuthMux.Get("/webhooks", webhooks)
	reqAuthMux.Post("/webhooks/append", webhooksAppend)
	reqAuthMux.Post("/webhooks/update/:id", webhooksUpdate)
	reqAuthMux.Post("/webhooks/delete/:id", webhooksDelete)
	goji.Post("/webhooks/fire/:name", webhookEvent)

	reqAuthMux.Get("/daily_tasks", dailyTasks)
	reqAuthMux.Post("/daily_tasks/append", dailyTasksAppend)
	reqAuthMux.Post("/daily_tasks/update/:id", dailyTasksUpdate)
	reqAuthMux.Post("/daily_tasks/delete/:id", dailyTasksDelete)

	reqAuthMux.Get("/update_proc_config_sets", updateProcConfigSets)
	reqAuthMux.Get("/regenerate_profiles", regenerateProfiles)

	reqAuthMux.Get("/system_logs", showMiniLogs)

	goji.Get("/information", showInfo)

	goji.Get("/api/profiles", showProfilesAPI)
	goji.Handle("/*", reqAuthMux)

	goji.Serve()
}


func index(c web.C, w http.ResponseWriter, r *http.Request) {
	tpl, err := pongo2.DefaultSet.FromFile("index.html")
	if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

	tasksForDisplay := gSubakoCtx.RunningTasks.MakeDisplayTask()

	tpl.ExecuteWriter(pongo2.Context{
		"config_sets_ctx": gSubakoCtx.ProcConfigSetsCtx,
		"tasks": tasksForDisplay,
		"queued_tasks": gSubakoCtx.QueueHelper,
	}, w)
}

func liveStatus(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("Running Task Id => %s\n", c.URLParams["id"])
	id, err := strconv.ParseInt(c.URLParams["id"], 10, 32)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusInternalServerError)
		return
	}

	runningTask := gSubakoCtx.RunningTasks.Get(int(id))
	if runningTask == nil {
		http.Error(w, "task is nil", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")		// important

	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
    if !ok {
		// ERROR
		http.Error(w, "Failed to cast to http.Flusher", http.StatusInternalServerError)
    }

	t, err := tail.TailFile(runningTask.LogFilePath, tail.Config{ Follow: true })
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// finish when timeout
	go func() {
		time.Sleep(time.Duration(60)*time.Second)	// timeout: 60sec
		t.Stop()
	}()
	// finish when task finished
	go func() {
		for {
			if !runningTask.IsActive() {
				t.Stop()
				break
			}
			time.Sleep(time.Duration(1)*time.Second)
		}
	}()
	// show logs
	for line := range t.Lines {
		fmt.Fprintln(w, line.Text)
		flusher.Flush() // Trigger "chunked" encoding and send a chunk...
	}

	fmt.Fprintf(w, "Current Status => %s\n", runningTask.Status)
	log.Printf("Task %d has been finished!!!!", runningTask.Id)
}

func status(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("Running Task Id => %s\n", c.URLParams["id"])
	id, err := strconv.ParseInt(c.URLParams["id"], 10, 32)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusInternalServerError)
		return
	}

	runningTask := gSubakoCtx.RunningTasks.Get(int(id))
	if runningTask == nil {
		http.Error(w, "task is nil", http.StatusInternalServerError)
		return
	}

	if runningTask.IsActive() {
		http.Error(w, "task is now active", http.StatusInternalServerError)
		return
	}

	buffer, err := ioutil.ReadFile(runningTask.LogFilePath)
	if err != nil {
		http.Error(w, "Failed to read logfile", http.StatusInternalServerError)
		return
	}

	tpl, err := pongo2.DefaultSet.FromFile("status.html")
	if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

	tpl.ExecuteWriter(pongo2.Context{
		"task": runningTask,
		"buffer": string(buffer),
	}, w)
}

func abortTask(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("Running Task Id => %s\n", c.URLParams["id"])
	id, err := strconv.ParseInt(c.URLParams["id"], 10, 32)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusInternalServerError)
		return
	}

	runningTask := gSubakoCtx.RunningTasks.Get(int(id))
	if runningTask == nil {
		http.Error(w, "task is nil", http.StatusInternalServerError)
		return
	}

	if runningTask.Killable() {
		if err := runningTask.Abort(); err != nil {
			http.Error(w, "Failed to kill container", http.StatusInternalServerError)
			return
		}

		// succeeded
		http.Redirect(w, r, "/", http.StatusFound)
		return

	} else {
		http.Error(w, "Container is not killable", http.StatusInternalServerError)
		return
	}
}


func build(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("build name => %s\n", c.URLParams["name"])
	log.Printf("build version => %s\n", c.URLParams["version"])

	name := c.URLParams["name"]
	version := c.URLParams["version"]

	procConfig, err := gSubakoCtx.ProcConfigSetsCtx.Find(name, version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	runningTask := gSubakoCtx.BuildAsync(procConfig)

	url := fmt.Sprintf("/live_status/%d", runningTask.Id)
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func queue(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("build name => %s\n", c.URLParams["name"])
	log.Printf("build version => %s\n", c.URLParams["version"])

	name := c.URLParams["name"]
	version := c.URLParams["version"]

	procConfig, err := gSubakoCtx.ProcConfigSetsCtx.Find(name, version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := gSubakoCtx.Queue(procConfig); err != nil {
		http.Error(w, "Failed to add the task to queue", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}


func showPackages(c web.C, w http.ResponseWriter, r *http.Request) {
	tpl, err := pongo2.DefaultSet.FromFile("packages.html")
	if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

	tpl.ExecuteWriter(pongo2.Context{
		"last_update": time.Unix(gSubakoCtx.AvailablePackages.LastUpdated, 0).String(),
		"packages": gSubakoCtx.AvailablePackages,
	}, w)
}

func removePackage(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("build name => %s\n", c.URLParams["name"])
	log.Printf("build version => %s\n", c.URLParams["version"])

	name := c.URLParams["name"]
	version := c.URLParams["version"]

	if err := gSubakoCtx.RemovePackage(name, version); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/packages", http.StatusSeeOther)
}


// Webhook called from other services
func webhookEvent(c web.C, w http.ResponseWriter, r *http.Request) {
	entryMsg := fmt.Sprintf("Webhook name => %s\n", c.URLParams["name"])
	log.Printf(entryMsg)
	gSubakoCtx.Logger.Succeeded(entryMsg)

	// get webhook task from database
	hook, err := gSubakoCtx.Webhooks.GetByTarget(c.URLParams["name"])
	if err != nil {
		msg := fmt.Sprintf("Failed to get the webhook task. %s", err)
		log.Printf(msg)
		gSubakoCtx.Logger.Failed("webhook", msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	if hook.Secret == "" {
		// DO NOTHING
		gSubakoCtx.Logger.Succeeded("webhook: do nothing")
		return
	}

	// read payload sent by github
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		msg := fmt.Sprintf("Failed to read the request body. %s", err)
		log.Printf(msg)
		gSubakoCtx.Logger.Failed("webhook", msg)
		http.Error(w, msg, http.StatusInternalServerError)
        return
	}

	payload := func() string {
		// TODO: support urlencoded
		return string(body)
	}()

	githubSig := r.Header.Get("X-Hub-Signature")
	log.Printf("payload => %s\n", payload)
	log.Printf("signature => %s\n", githubSig)

	// generate hash
	mac := hmac.New(sha1.New, []byte(hook.Secret))
	mac.Write([]byte(payload))
	expectedMAC := mac.Sum(nil)
	log.Printf("expected MAC => %s\n", hex.EncodeToString(expectedMAC))
	generatedSig := "sha1=" + hex.EncodeToString(expectedMAC)

	if githubSig != generatedSig {
		msg := "Invalid signature"
		log.Printf(msg)
		gSubakoCtx.Logger.Failed("webhook", msg)
		http.Error(w, msg, http.StatusInternalServerError)
	}

	// special
	if c.URLParams["name"] == packageConfigsHookName {
		if err := gSubakoCtx.RefreshProfileConfigs(); err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		return
	}

	// queue target script
	procConfig, err := gSubakoCtx.ProcConfigSetsCtx.Find(hook.ProcName, hook.Version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		gSubakoCtx.Logger.Failed("webhook", err.Error())
		return
	}

	if err := gSubakoCtx.Queue(procConfig); err != nil {
		msg := "Failed to add the task to queue"
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	// succeeded
	gSubakoCtx.Logger.Succeeded("webhook")
}

func webhooks(c web.C, w http.ResponseWriter, r *http.Request) {
	webhooks := gSubakoCtx.Webhooks.GetWebhooks()

	tpl, err := pongo2.DefaultSet.FromFile("webhooks.html")
	if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

	tpl.ExecuteWriter(pongo2.Context{
		"webhooks": webhooks,
	}, w)
}

func webhooksAppend(c web.C, w http.ResponseWriter, r *http.Request) {
	if r.FormValue("target") == "" {
		http.Error(w, "target is empty", http.StatusInternalServerError)
		return
	}
	if r.FormValue("secret") == "" {
		http.Error(w, "secret is empty", http.StatusInternalServerError)
		return
	}
	if r.FormValue("proc_name") == "" {
		http.Error(w, "proc_name is empty", http.StatusInternalServerError)
		return
	}
	if r.FormValue("version") == "" {
		http.Error(w, "version is empty", http.StatusInternalServerError)
		return
	}

	hook := &subako.Webhook{
		Target: r.FormValue("target"),
		Secret: r.FormValue("secret"),
		ProcName: r.FormValue("proc_name"),
		Version: r.FormValue("version"),
	}

	if err := gSubakoCtx.Webhooks.Append(hook); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/webhooks", http.StatusFound)
}

func webhooksUpdate(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("Webhook Id => %s\n", c.URLParams["id"])
	id, err := strconv.ParseUint(c.URLParams["id"], 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusInternalServerError)
		return
	}

	if r.FormValue("target") == "" {
		http.Error(w, "target is empty", http.StatusInternalServerError)
		return
	}
	if r.FormValue("secret") == "" {
		http.Error(w, "secret is empty", http.StatusInternalServerError)
		return
	}
	if r.FormValue("proc_name") == "" {
		http.Error(w, "proc_name is empty", http.StatusInternalServerError)
		return
	}
	if r.FormValue("version") == "" {
		http.Error(w, "version is empty", http.StatusInternalServerError)
		return
	}

	hook := &subako.Webhook{
		Target: r.FormValue("target"),
		Secret: r.FormValue("secret"),
		ProcName: r.FormValue("proc_name"),
		Version: r.FormValue("version"),
	}

	if err := gSubakoCtx.Webhooks.Update(uint(id), hook); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/webhooks", http.StatusFound)
}

func webhooksDelete(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("Webhook Id => %s\n", c.URLParams["id"])
	id, err := strconv.ParseUint(c.URLParams["id"], 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusInternalServerError)
		return
	}

	if err := gSubakoCtx.Webhooks.Delete(uint(id)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/webhooks", http.StatusFound)
}


func dailyTasks(c web.C, w http.ResponseWriter, r *http.Request) {
	tasks := gSubakoCtx.DailyTasks.GetDailyTasks()

	tpl, err := pongo2.DefaultSet.FromFile("daily_tasks.html")
	if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

	tpl.ExecuteWriter(pongo2.Context{
		"tasks": tasks,
		"point": gSubakoCtx.DailyTasks.Point,
		"now": time.Now(),
	}, w)
}

func dailyTasksAppend(c web.C, w http.ResponseWriter, r *http.Request) {
	if r.FormValue("proc_name") == "" {
		http.Error(w, "proc_name is empty", http.StatusInternalServerError)
		return
	}
	if r.FormValue("version") == "" {
		http.Error(w, "version is empty", http.StatusInternalServerError)
		return
	}

	task := &subako.DailyTask{
		ProcName: r.FormValue("proc_name"),
		Version: r.FormValue("version"),
	}

	if err := gSubakoCtx.DailyTasks.Append(task); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/daily_tasks", http.StatusFound)
}

func dailyTasksUpdate(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("DailyTask Id => %s\n", c.URLParams["id"])
	id, err := strconv.ParseUint(c.URLParams["id"], 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusInternalServerError)
		return
	}

	if r.FormValue("proc_name") == "" {
		http.Error(w, "proc_name is empty", http.StatusInternalServerError)
		return
	}
	if r.FormValue("version") == "" {
		http.Error(w, "version is empty", http.StatusInternalServerError)
		return
	}

	task := &subako.DailyTask{
		ProcName: r.FormValue("proc_name"),
		Version: r.FormValue("version"),
	}

	if err := gSubakoCtx.DailyTasks.Update(uint(id), task); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/daily_tasks", http.StatusFound)
}

func dailyTasksDelete(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("DailyTask Id => %s\n", c.URLParams["id"])
	id, err := strconv.ParseUint(c.URLParams["id"], 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusInternalServerError)
		return
	}

	if err := gSubakoCtx.DailyTasks.Delete(uint(id)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/daily_tasks", http.StatusFound)
}


func updateProcConfigSets(c web.C, w http.ResponseWriter, r *http.Request) {
	if err := gSubakoCtx.RefreshProfileConfigs(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func regenerateProfiles(c web.C, w http.ResponseWriter, r *http.Request) {
	if err := gSubakoCtx.UpdateProfilesWithNotification(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}


func showMiniLogs(c web.C, w http.ResponseWriter, r *http.Request) {
	tpl, err := pongo2.DefaultSet.FromFile("system_logs.html")
	if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

	latestLogs := gSubakoCtx.Logger.GetLatest(50)

	tpl.ExecuteWriter(pongo2.Context{
		"latest_logs": latestLogs,
	}, w)
}


func showInfo(c web.C, w http.ResponseWriter, r *http.Request) {
	tpl, err := pongo2.DefaultSet.FromFile("information.html")
	if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

	tpl.ExecuteWriter(pongo2.Context{}, w)
}


func showProfilesAPI(c web.C, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	profiles := gSubakoCtx.Profiles.Profiles

	encoder := json.NewEncoder(w)
    encoder.Encode(profiles)
}
