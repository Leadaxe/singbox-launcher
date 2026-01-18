package process

import (
	"github.com/mitchellh/go-ps"
)

// ProcessInfo is a small struct representing a running process.
// It is intentionally minimal to keep cross-platform compatibility.
type ProcessInfo struct {
	PID  int
	Name string
}

// GetProcesses returns a list of running processes in a platform-agnostic format.
// It wraps github.com/mitchellh/go-ps internally and normalizes the result.
func GetProcesses() ([]ProcessInfo, error) {
	procs, err := ps.Processes()
	if err != nil {
		return nil, err
	}
	out := make([]ProcessInfo, 0, len(procs))
	for _, p := range procs {
		out = append(out, ProcessInfo{PID: p.Pid(), Name: p.Executable()})
	}
	return out, nil
}

// FindProcess looks up a process by PID and returns it with a boolean indicating whether it was found.
func FindProcess(pid int) (ProcessInfo, bool, error) {
	procs, err := GetProcesses()
	if err != nil {
		return ProcessInfo{}, false, err
	}
	for _, p := range procs {
		if p.PID == pid {
			return p, true, nil
		}
	}
	return ProcessInfo{}, false, nil
}
