package main

import (
	"bufio"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/chzyer/readline"
)

const exitcmd = "exit"

var (
	releaseVersion string
	debugmode      bool
	noprepull      bool
	customkubectl  string
	prevdir        string
	rl             *readline.Instance
	completer      = readline.NewPrefixCompleter(
		readline.PcItem("cat"),
		readline.PcItem("cd"),
		readline.PcItem("curl"),
		readline.PcItem("contexts"),
		readline.PcItem("echo"),
		readline.PcItem("env",
			readline.PcItem("list"),
			readline.PcItem("create"),
			readline.PcItem("select"),
			readline.PcItem("delete")),
		readline.PcItem(exitcmd),
		readline.PcItem("help"),
		readline.PcItem("kill"),
		readline.PcItem("literally"),
		readline.PcItem("ls"),
		readline.PcItem("ps", readline.PcItem("all")),
		readline.PcItem("pwd"),
		readline.PcItem("sleep"),
		readline.PcItem("use"),
	)
)

func init() {
	if env := os.Getenv("KUBEDSH_DEBUG"); env != "" {
		debugmode = true
	}
	if env := os.Getenv("KUBEDSH_NOPREPULL"); env != "" {
		noprepull = true
	}
	if env := os.Getenv("KUBECTL_BINARY"); env != "" {
		customkubectl = env
	}
	prevdir, _ = os.Getwd()
	// set up the global distributed process table:
	dpt = &DProcTable{
		mux: new(sync.Mutex),
		lt:  make(map[string]DProc),
	}
	err := dpt.BuildDPT()
	if err != nil {
		output(err.Error())
	}
}

func main() {
	var script string
	// first, check if we've got a script filename
	// passed in via command line argument:
	if len(os.Args) == 2 {
		scriptfile := os.Args[1]
		b, err := ioutil.ReadFile(scriptfile)
		if err != nil {
			warn("Error executing script: " + err.Error())
		}
		script = string(b)
		interprets(script)
		return
	}
	// now let's see if we maybe have a script via stdin:
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			script += scanner.Text() + "\n"
		}
		if scanner.Err() != nil {
			warn("Error reading from stdin: " + scanner.Err().Error())
		}
		interprets(script)
		return
	}
	// well seems we're gonna be running interactive:
	err := preflight()
	if err != nil {
		warn("Encountered issues during startup: " + err.Error())
	}
	rl, err = readline.NewEx(&readline.Config{
		AutoComplete:    completer,
		HistoryFile:     "/tmp/readline.tmp",
		InterruptPrompt: "^C",
	})
	if err != nil {
		warn("Encountered issues during startup: " + err.Error())
	}
	defer func() {
		_ = rl.Close()
	}()
	// create and select global environment
	createenv(globalEnv, false)
	err = selectenv(globalEnv, false)
	if err != nil {
		warn("Encountered issues during startup: " + err.Error())
	}
	log.SetOutput(rl.Stderr())
	output("\nType 'help' to learn about available built-in commands.")
	// set up hotreload watchdog:
	rwatch = &ReloadWatchdog{}
	rwatch.init(currentenv().evt)
	go rwatch.run()
	// kick off garbage collection:
	go gcDProcs()
	// necessary hack to make readline ignore a cascaded CTRL+C:
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			debug("caught an cascaded CTRL+C, ignoring it")
		}
	}()
	// kick off main interactive interpreter loop:
	interpreti(rl)
}
