package main

import (
	"subako"

	"os"
	"log"

	"net/http"

	"io/ioutil"

	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"

	"github.com/flosch/pongo2"
	"github.com/ActiveState/tail"

	"strconv"
	"encoding/json"

	"time"
	"path"
	"fmt"

	"crypto/hmac"
	"crypto/sha1"

	"encoding/hex"
)

var gSubakoCtx *subako.SubakoContext

func main() {
	defer func() {
		log.Println("Exit main")
	}()

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	config := &subako.SubakoConfig{
		ProcConfigSetsBaseDir: path.Join(cwd, "../proc_configs_test"),
		AvailablePackagesPath: path.Join(cwd, "../available_packages.json"),
		AptRepositoryBaseDir: path.Join(cwd, "../apt_repository"),
		VirtualUsrDir: path.Join(cwd, "../torigoya_usr"),
		TmpBaseDir: path.Join(cwd, "../temp"),
		PackagesDir: path.Join(cwd, "../packages"),
		RunningTasksPath: path.Join(cwd, "../running_tasks.json"),
		ProfilesHolderPath: path.Join(cwd, "../proc_profiles.json"),
		DataBasePath: path.Join(cwd, "../db.sqlite"),
		UpdatedNotificationURL: "http://localhost:3000/",
		CronData: subako.Crontab {
			Hour: 2,
			Minute: 50,
		},
		LogDir: "/tmp",
	}

	subakoCtx, err := subako.MakeSubakoContext(config)
	if err != nil {
		panic("error")
	}
	defer func() {
		if err := subakoCtx.Save(); err != nil {
			panic(err)
		}
	}()

	gSubakoCtx = subakoCtx

	//
	pongo2.DefaultSet.SetBaseDirectory("views")

	goji.Get("/assets/*", http.StripPrefix("/assets/", http.FileServer(http.Dir("./public"))))
	goji.Get("/apt/*", http.StripPrefix("/apt/", http.FileServer(http.Dir(subakoCtx.AptRepoCtx.AptRepositoryBaseDir))))

	goji.Get("/", index)

	goji.Get("/live_status/:id", liveStatus)
	goji.Get("/status/:id", status)

	goji.Get("/build/:name/:version", build)
	goji.Get("/queue/:name/:version", queue)

	goji.Get("/packages", showPackages)

	goji.Get("/webhooks", webhooks)
	goji.Post("/webhooks/:name", webhookEvent)
	goji.Post("/webhooks/append", webhooksAppend)
	goji.Post("/webhooks/update/:id", webhooksUpdate)
	goji.Post("/webhooks/delete/:id", webhooksDelete)

	goji.Get("/api/profiles", showProfilesAPI)

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
		"config_sets": gSubakoCtx.ProcConfigSets,
		"tasks": tasksForDisplay,
		"queued_tasks": gSubakoCtx.QueueHelper,
	}, w)
}

func liveStatus(c web.C, w http.ResponseWriter, r *http.Request) {
	// TODO: add authentication

	log.Printf("Running Task Id => %s\n", c.URLParams["id"])
	id, err := strconv.ParseInt(c.URLParams["id"], 10, 32)
	if err != nil {
		// ERROR
		return
	}

	runningTask := gSubakoCtx.RunningTasks.Get(int(id))
	if runningTask == nil {
		// ERROR
		return
	}

	// if task has been already finished, move to static status page
	if !runningTask.IsActive {
		url := fmt.Sprintf("/status/%d", runningTask.Id)
		http.Redirect(w, r, url, http.StatusMovedPermanently)

		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")		// important

	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
    if !ok {
		// ERROR
		panic("expected http.ResponseWriter to be an http.Flusher")
    }

	t, err := tail.TailFile(runningTask.LogFilePath, tail.Config{ Follow: true })
	if err != nil {
		// ERROR
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
			if !runningTask.IsActive {
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
	// TODO: add authentication

	log.Printf("Running Task Id => %s\n", c.URLParams["id"])
	id, err := strconv.ParseInt(c.URLParams["id"], 10, 32)
	if err != nil {
		return
	}

	runningTask := gSubakoCtx.RunningTasks.Get(int(id))
	if runningTask == nil {
		// ERROR
		return
	}

	// if task has been already finished, move to static status page
	if runningTask.IsActive {
		// ERROR
		return
	}

	buffer, err := ioutil.ReadFile(runningTask.LogFilePath)
	if err != nil {
		// ERROR
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


func build(c web.C, w http.ResponseWriter, r *http.Request) {
	// TODO: add authentication

	log.Printf("build name => %s\n", c.URLParams["name"])
	log.Printf("build version => %s\n", c.URLParams["version"])

	name := c.URLParams["name"]
	version := c.URLParams["version"]

	procConfig, err := gSubakoCtx.FindProcConfig(name, version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	runningTask := gSubakoCtx.BuildAsync(procConfig)

	url := fmt.Sprintf("/live_status/%d", runningTask.Id)
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func queue(c web.C, w http.ResponseWriter, r *http.Request) {
	// TODO: add authentication

	log.Printf("build name => %s\n", c.URLParams["name"])
	log.Printf("build version => %s\n", c.URLParams["version"])

	name := c.URLParams["name"]
	version := c.URLParams["version"]

	procConfig, err := gSubakoCtx.FindProcConfig(name, version)
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
		"packages": gSubakoCtx.AvailablePackages,
	}, w)
}

// Webhook called from other services
func webhookEvent(c web.C, w http.ResponseWriter, r *http.Request) {
	log.Printf("Webhook name => %s\n", c.URLParams["name"])

	// get webhook task from database
	hook, err := gSubakoCtx.Webhooks.GetByTarget(c.URLParams["name"])
	if err != nil {
		msg := fmt.Sprintf("Failed to get the webhook task. %s", err)
		log.Printf(msg)
		http.Error(w, msg, http.StatusInternalServerError)
        return
	}

	// read payload sent by github
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		msg := fmt.Sprintf("Failed to read the request body. %s", err)
		log.Printf(msg)
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
		http.Error(w, msg, http.StatusInternalServerError)
	}

	// queue target script
	procConfig, err := gSubakoCtx.FindProcConfig(hook.ProcName, hook.Version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := gSubakoCtx.Queue(procConfig); err != nil {
		http.Error(w, "Failed to add the task to queue", http.StatusInternalServerError)
		return
	}

	// succeeded
}

func webhooks(c web.C, w http.ResponseWriter, r *http.Request) {
	// TODO: Add authentication
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


func showProfilesAPI(c web.C, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	profiles := gSubakoCtx.Profiles.Profiles

	encoder := json.NewEncoder(w)
    encoder.Encode(profiles)
}
