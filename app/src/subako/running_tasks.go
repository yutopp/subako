package subako

import (
	"log"
	"sync"
	"errors"
	"os"
)


const maxShowingTaskNum = 30

type RunningStatus int
const (
	TaskRunning = RunningStatus(0)
	TaskSucceeded = RunningStatus(1)
	TaskFailed = RunningStatus(2)
	TaskAborted = RunningStatus(3)
	TaskWarning = RunningStatus(4)
)

func (s RunningStatus) String() string {
	switch s {
	case TaskRunning:
		return "Running..."
	case TaskSucceeded:
		return "Succeeded"
	case TaskFailed:
		return "Failed"
	case TaskAborted:
		return "Aborted"
	case TaskWarning:
		return "Warning"
	}
	return ""
}

type RunningTask struct {
	Id					int
	LogName				string
	LogFilePath			string
	Status				RunningStatus
	ErrorText			string

	ContainerID			*string			`json:"-"`	// ignore when saving
	KillContainer		*func() error	`json:"-"`	// ignore when saving
}

func (rt *RunningTask) GetError() error {
	if rt.ErrorText == "" {
		return nil
	} else {
		return errors.New(rt.ErrorText)
	}
}

func (rt *RunningTask) IsActive() bool {
	return rt.Status == TaskRunning
}

func (rt *RunningTask) Killable() bool {
	return rt.IsActive() && rt.ContainerID != nil && rt.KillContainer != nil
}

func (rt *RunningTask) Warning(message string) {
	if rt.Status == TaskRunning {
		rt.Status = TaskWarning
	}
	rt.ErrorText = message
}

func (rt *RunningTask) Failed(message string) {
	if rt.Status == TaskRunning {
		rt.Status = TaskFailed
	}
	rt.ErrorText = message
}

func (rt *RunningTask) Abort() error {
	var err error

	if rt.Killable() {
		err = (*rt.KillContainer)()
	}

	rt.Status = TaskAborted

	return err
}


type RunningTasks struct {
	Next		int
	Tasks		[]*RunningTask

	m			sync.Mutex	`json:"-"`	// ignore when saving
	HasFilePath
}

func LoadRunningTasks(path string) (*RunningTasks, error) {
	var rt RunningTasks
	if err := LoadStructure(path, &rt); err != nil {
		return nil, err
	}

	return &rt, nil
}

func (rt *RunningTasks) Save() error {
	rt.m.Lock()
	defer rt.m.Unlock()

	// throw away...
	if len(rt.Tasks) > maxShowingTaskNum {
		ln := len(rt.Tasks)
		for _, task := range rt.Tasks[:ln-maxShowingTaskNum] {
			log.Printf("deleting log file => %s", task.LogFilePath)
			os.Remove(task.LogFilePath)
		}

		rt.Tasks = rt.Tasks[ln-maxShowingTaskNum:ln]
		for i, task := range rt.Tasks {
			task.Id = i
		}
		rt.Next = maxShowingTaskNum
	}

	// kill running tasks
	for _, task := range rt.Tasks {
		if task.Status == TaskRunning {
			task.Abort()
		}
	}

	return SaveStructure(rt)
}

func (rt *RunningTasks) createTaskHolder() *RunningTask {
	rt.m.Lock()
	defer rt.m.Unlock()

	if rt.Tasks == nil {
		rt.Tasks = make([]*RunningTask, 0, 100)
	}
	task := &RunningTask{
		Id: rt.Next,
	}
	rt.Tasks = append(rt.Tasks, task)
	rt.Next++

	return task
}

func (rt *RunningTasks) Get(id int) *RunningTask {
	rt.m.Lock()
	defer rt.m.Unlock()

	if rt.Tasks == nil { return nil }
	if id >= len(rt.Tasks) { return nil }

	return rt.Tasks[id]
}

func (rt *RunningTasks) MakeDisplayTask() []*RunningTask {
	rt.m.Lock()
	defer rt.m.Unlock()
	num := minI(rt.Next, 30)

	arr := make([]*RunningTask, num)
	if rt.Next == 0 { return arr }

	for i := 0; i < num; i++ {
		ti := rt.Next - i - 1
		// log.Printf("DisplayTask %d", ti)
		arr[i] = rt.Tasks[ti]
	}

	return arr
}
