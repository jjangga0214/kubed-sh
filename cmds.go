package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/chzyer/readline"
)

func hcurl(line string) {
	if !strings.ContainsAny(line, " ") {
		info("Need a target URL, for example `curl someservice` in the cluster or `curl http://example.com`")
		return
	}
	url := strings.Split(line, " ")[1]
	res, err := kubectl(true, "run", "-i", "-t", "--rm", "curljump", "--restart=Never",
		"--image=quay.io/mhausenblas/jump:v0.1", "--", "curl", url)
	if err != nil {
		warn(fmt.Sprintf("Can't curl %s due to: %s", url, err.Error()))
		return
	}
	output(res)
}

func hlocalexec(line string) {
	cmd := line
	args := []string{}
	if strings.ContainsAny(line, " ") {
		cmd = strings.Split(line, " ")[0]
		args = strings.Split(line, " ")[1:]
	}
	res, err := shellout(true, cmd, args...)
	if err != nil {
		fmt.Printf("Failed to execute %s locally due to: %s", cmd, err)
	}
	output(res)
}

func henv() {
	tmp := []string{}
	for k, v := range evt.et {
		tmp = append(tmp, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(tmp)
	for _, e := range tmp {
		output(e)
	}
}

func hkill(line string) {
	if !strings.ContainsAny(line, " ") {
		info("Need a target distributed process to kill")
		return
	}
	// pre-flight check if dproc exists:
	ID := strings.Split(line, " ")[1]
	_, err := kubectl(true, "get", "deployment", ID)
	if err != nil {
		warn(fmt.Sprintf("A distributed process with the ID '%s' does not exist in current context. Try the ps command …", ID))
		return
	}
	_, err = kubectl(true, "scale", "--replicas=0", "deployment", ID)
	if err != nil {
		killfail(line, err.Error())
		return
	}
	_, err = kubectl(true, "delete", "deployment", ID)
	if err != nil {
		killfail(line, err.Error())
		return
	}

	// gather info to remove from global DPT:
	kubecontext, err := kubectl(true, "config", "current-context")
	if err != nil {
		killfail(line, err.Error())
		return
	}
	dproc, err := dpt.getDProc(ID, kubecontext)
	if err != nil {
		killfail(line, err.Error())
		return
	}
	// something like xxx:blah
	src := strings.Split(dproc.Src, ":")[1]
	// now get rid of the extension:
	svcname := src[0 : len(src)-len(filepath.Ext(src))]
	_, err = kubectl(true, "delete", "service", svcname)
	if err != nil {
		killfail(line, err.Error())
		return
	}
	if err != nil {
		info(err.Error())
	}
	// finally, remove drpoc from global DPT:
	dpt.removeDProc(dproc)
}

func hps(line string) {
	args := ""
	if strings.ContainsAny(line, " ") {
		args = strings.Split(line, " ")[1]
	}
	var kubecontext string
	switch args {
	case "all":
		kubecontext = ""
	default:
		k, err := kubectl(true, "config", "current-context")
		if err != nil {
			warn("Can't determine current context")
			return
		}
		kubecontext = k
	}
	res := dpt.DumpDPT(kubecontext)
	output(res)
}

func huse(line string, rl *readline.Instance) {
	if !strings.ContainsAny(line, " ") {
		info("Need a target cluster")
		return
	}
	targetcontext := strings.Split(line, " ")[1]
	res, err := kubectl(true, "config", "use-context", targetcontext)
	if err != nil {
		fmt.Printf("\nFailed to switch contexts due to:\n%s\n\n", err)
		return
	}
	output(res)
	setprompt(rl, targetcontext)
}

func hcontexts() {
	res, err := kubectl(true, "config", "get-contexts")
	if err != nil {
		fmt.Printf("\nFailed to list contexts due to:\n%s\n\n", err)
	}
	output(res)
}

func launchfail(line, reason string) {
	fmt.Printf("\nFailed to launch %s in the cluster due to:\n%s\n\n", strconv.Quote(line), reason)
}

func killfail(line, reason string) {
	fmt.Printf("\nFailed to kill %s due to:\n%s\n\n", strconv.Quote(line), reason)
}

func hlaunch(line string) {
	// If a line doesn't start with one of the
	// known environments, assume user wants to
	// launch a binary:
	var dpid string
	src := extractsrc(line)
	src = "script:" + src
	switch {
	case strings.HasPrefix(line, "python "):
		d, err := launchpy(line)
		if err != nil {
			launchfail(line, err.Error())
			return
		}
		dpid = d
	case strings.HasPrefix(line, "node "):
		d, err := launchjs(line)
		if err != nil {
			launchfail(line, err.Error())
			return
		}
		dpid = d
	case strings.HasPrefix(line, "ruby "):
		d, err := launchrb(line)
		if err != nil {
			launchfail(line, err.Error())
			return
		}
		dpid = d
	default: // binary
		d, err := launch(line)
		if err != nil {
			launchfail(line, err.Error())
			return
		}
		dpid = d
		src = "bin:" + extractsrc(line)
	}
	// update DPT
	if strings.HasSuffix(line, "&") {
		kubecontext, err := kubectl(true, "config", "current-context")
		if err != nil {
			launchfail(line, err.Error())
			return
		}
		dpt.addDProc(newDProc(dpid, DProcLongRunning, kubecontext, src))
	}
}

func hliterally(line string) {
	if !strings.ContainsAny(line, " ") {
		info("Not enough input for a valid kubectl command")
		return
	}
	l := strings.Split(line, " ")
	res, _ := kubectl(true, l[1], l[2:]...)
	output(res)
}

func hecho(line string) {
	if !strings.ContainsAny(line, " ") {
		info("No value to echo given")
		return
	}
	echo := strings.Split(line, " ")[1]
	if strings.HasPrefix(echo, "$") {
		v := evt.get(echo[1:])
		if v != "" {
			fmt.Println(v)
			return
		}
	}
	fmt.Println(echo)
}