//go:build windows

package plugins

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"sync"
	"unsafe"
)

var nativeJobs sync.Map

func prepareNativeProcess(cmd *exec.Cmd) func() error {
	return func() error {
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return err
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(job)
			return err
		}
		process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
		if err != nil {
			windows.CloseHandle(job)
			return err
		}
		defer windows.CloseHandle(process)
		if err = windows.AssignProcessToJobObject(job, process); err != nil {
			windows.CloseHandle(job)
			return err
		}
		nativeJobs.Store(cmd, job)
		return nil
	}
}
func terminateNativeTree(cmd *exec.Cmd) {
	if job, ok := nativeJobs.LoadAndDelete(cmd); ok {
		windows.CloseHandle(job.(windows.Handle))
	}
}
