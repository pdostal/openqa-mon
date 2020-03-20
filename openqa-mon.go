package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

// Terminal color codes
const KNRM = "\x1B[0m"
const KRED = "\x1B[31m"
const KGRN = "\x1B[32m"
const KYEL = "\x1B[33m"
const KBLU = "\x1B[34m"
const KMAG = "\x1B[35m"
const KCYN = "\x1B[36m"
const KWHT = "\x1B[37m"

// Job is a running Job instance
type Job struct {
	AssignedWorkerID int `json:"assigned_worker_id"`
	BlockedByID      int `json:"blocked_by_id"`
	// Children
	CloneID int `json:"clone_id"`
	GroupID int `json:"group_id"`
	ID      int `json:"id"`
	// Modules
	Name string `json:"name"`
	// Parents
	Priority  int      `json:"priority"`
	Result    string   `json:"result"`
	Settings  Settings `json:"settings"`
	State     string   `json:"state"`
	Tfinished string   `json:"t_finished"`
	Tstarted  string   `json:"t_started"`
	Test      string   `json:"test"`
}

type JobStruct struct {
	Job Job `json:"job"`
}

type Jobs struct {
	Jobs []Job `json:"jobs"`
}

type Settings struct {
	Arch    string `json:"ARCH"`
	Backend string `json:"BACKEND"`
	Machine string `json:"MACHINE"`
}

func ensureHTTP(remote string) string {
	if !(strings.HasPrefix(remote, "http://") || strings.HasPrefix(remote, "https://")) {
		return "http://" + remote
	} else {
		return remote
	}
}

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func terminalWidth() int {
	ws := &winsize{}
	ret, _, _ := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)))

	if int(ret) == 0 {
		return int(ws.Col)
	} else {
		return 80 // Default value
	}
}

// Println prints the current job in a 80 character wide line with optional colors enabled
func (job *Job) Println(useColors bool, width int) {
	name := job.Test + "@" + job.Settings.Machine

	// Crop or extend name, so that the total line is filled. We need 25 characters for id, progress ecc.
	if width < 50 {
		width = 50
	}
	if len(name) > width-25 {
		fmt.Printf("%s %d %d\n", name, len(name), width-25)
		name = name[:width-25]
	}
	for len(name) < width-25 {
		name = name + " "
	}

	if job.State == "running" {
		if useColors {
			fmt.Print(KGRN)
		}
		fmt.Printf(" %-6d %s %15s\n", job.ID, name, job.State)
		if useColors {
			fmt.Print(KNRM)
		}
	} else if job.State == "done" {
		if useColors {
			switch job.Result {
			case "failed":
				fmt.Print(KRED)
			case "incomplete":
				fmt.Print(KRED)
			case "user_cancelled":
				fmt.Print(KYEL)
			case "passed":
				fmt.Print(KBLU)
			default:
				fmt.Print(KWHT)
			}
		}
		fmt.Printf(" %-6d %s %15s\n", job.ID, name, job.Result)
		if useColors {
			fmt.Print(KNRM)
		}
	} else {

		if useColors {
			fmt.Print(KCYN)
		}
		fmt.Printf(" %-6d %s %15s\n", job.ID, name, job.State)
		if useColors {
			fmt.Print(KNRM)
		}
	}

}

/* Struct for sorting job slice by job id */
type byID []Job

func (s byID) Len() int {
	return len(s)
}
func (s byID) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byID) Less(i, j int) bool {
	return s[i].ID < s[j].ID

}

func fetchJob(url string, jobID int) (Job, error) {
	var job JobStruct
	url = fmt.Sprintf("%s/api/v1/jobs/%d", url, jobID)
	resp, err := http.Get(url)
	if err != nil {
		return job.Job, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return job.Job, err
	}
	err = json.Unmarshal(body, &job)
	if err != nil {
		return job.Job, err
	}

	return job.Job, nil
}

func getJobsOverview(url string) ([]Job, error) {
	var jobs []Job
	resp, err := http.Get(url + "/api/v1/jobs/overview")
	if err != nil {
		return jobs, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return jobs, err
	}
	err = json.Unmarshal(body, &jobs)

	// Fetch more details about the jobs
	for i, job := range jobs {
		job, err = fetchJob(url, job.ID)
		if err != nil {
			return jobs, err
		}
		jobs[i] = job
	}
	return jobs, nil
}

func printHelp() {
	fmt.Printf("Usage: %s REMOTE\n  REMOTE is the base URL of the openQA server (e.g. https://openqa.opensuse.org)\n", os.Args[0])
}

func main() {
	args := os.Args[1:]
	remotes := make([]string, 0)

	// Parse program arguments
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			continue
		}
		if arg[0] == '-' {
			switch arg {
			case "-h", "--help":
				printHelp()
				return
			default:
				fmt.Fprintf(os.Stderr, "Invalid argument: %s\n", arg)
				fmt.Printf("Use %s --help to display available options\n", os.Args[0])
				os.Exit(1)
			}
		} else {
			// Assume host
			remotes = append(remotes, arg)
		}
	}

	if len(remotes) == 0 {
		printHelp()
		return
	}

	termWidth := terminalWidth()
	useColors := true
	for _, remote := range remotes {
		remote = ensureHTTP(remote)

		jobs, err := getJobsOverview(remote)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching jobs: %s\n", err)
			continue
		}
		if len(jobs) == 0 {
			fmt.Println("No jobs on instance found")
			continue
		}
		// Sort jobs by ID
		sort.Sort(byID(jobs))

		for _, job := range jobs {
			job.Println(useColors, termWidth)
		}
	}

}
