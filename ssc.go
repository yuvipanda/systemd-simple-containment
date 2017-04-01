package main

import "flag"
import "os/exec"
import "syscall"
import "fmt"
import "os"
import "strings"
import "path/filepath"

// Run a given command with given args, uid, gid, systemd properties & env vars
func SystemdRun(
	path string,
	args []string,
	uid int,
	gid int,
	attachTty bool,
	properties map[string]string,
	environment map[string]string,
) {
	cmd := []string{
		"/usr/bin/systemd-run",
		// string(uid) produces weird results. WHY?!
		"--uid", fmt.Sprintf("%d", uid),
		"--gid", fmt.Sprintf("%d", gid),
		"--quiet",
	}

	if attachTty {
		cmd = append(cmd, "--tty")
	}

	for key, value := range properties {
		cmd = append(cmd, "-p", fmt.Sprintf("%s=%s", key, value))
	}

	for key, value := range environment {
		cmd = append(cmd, "--setenv", fmt.Sprintf("%s=%s", key, value))
	}

	cmd = append(cmd, path)
	cmd = append(cmd, args...)

	err := syscall.Exec("/usr/bin/systemd-run", cmd, []string{})

	if err != nil {
		panic(err)
	}
}

// Return a map of environment variables.
// Not sure why this isn't built into the language
// TODO: Investigate how environment variables really work
func GetEnvironMap() map[string]string {
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		// FIXME: This looks terrible?
		pair := strings.Split(e, "=")
		envMap[pair[0]] = pair[1]
	}

	return envMap
}

func main() {
	isolationPtr := flag.String("isolation", "strict", "How isolated you want the process to be. Current optionsare 'strict' or 'relaxed'")
	// Set this to false when using GUI applications, so they don't hog the bash session they might be launched from
	ttyPtr := flag.Bool("tty", true, "Set to false to have spawned process not inherit tty. Useful for GUIs")

	flag.Parse()

	// We separate the path to executable from args, so we can do $PATH based resolution on path
	// This allows people to run us like `ssc bash` without having to specify full path.
	path := flag.Arg(0)
	args := flag.Args()[1:]

	resolvedPath, err := exec.LookPath(path)
	if err != nil {
		panic(fmt.Sprintf("Could not find executable %v: %v", path, err))
	}

	// Now, would *this* ever fail? I've no idea, and I miss exceptions already
	absolutePath, err := filepath.Abs(resolvedPath)
	if err != nil {
		panic(fmt.Sprintf("Could not make absolute path to %v: %v", resolvedPath, err))
	}

	// Would this ever fail? Perhaps if wd is set to something that's deleted?
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	properties := map[string]string{
		// Add a CPUQuota here once https://github.com/systemd/systemd/issues/3851 gets fixed
		"MemoryMax":        "70%",
		"WorkingDirectory": cwd,
	}

	switch *isolationPtr {
	case "relaxed":
		break
	case "strict":
		properties["NoNewPrivileges"] = "yes"
		properties["PrivateTmp"] = "yes"
	default:
		fmt.Printf("Unsupported value for -isolation parameter: %s", *isolationPtr)
		os.Exit(1)
	}

	// Most important invariant here is to never allow exec'ing as anything other than
	// the calling user id / group id, since we will be a setuid binary owned by root &
	// hence running as root...
	SystemdRun(
		absolutePath,
		args,
		syscall.Getuid(),
		syscall.Getgid(),
		*ttyPtr,
		properties,
		GetEnvironMap(),
	)
}
