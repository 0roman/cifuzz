//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package executil

import (
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
)

// IsTerminatedExitErr returns true if the error is the expected error
// when the process was terminated via Cmd.TerminateProcessGroup and the
// process exited within the grace period.
//
// On Unix, multiple errors can happen when a SIGTERM is sent to the
// process group:
//   - The process can have exit code 143, which is the expected exit
//     code when a process receives a SIGTERM
//   - The process can have exit code -1 and the signal of the wait status
//     set to SIGTERM
//   - In case that the process doesn't handle the SIGTERM fast enough and
//     tries to write to the pipe which was already closed by
//     TerminateProcessGroup, the process has exit code -1 and the signal
//     of the wait status is set to SIGPIPE
func IsTerminatedExitErr(err error) bool {
	var exitErr *exec.ExitError
	errors.As(err, &exitErr)
	if exitErr.ExitCode() == 143 {
		return true
	}
	signal := exitErr.Sys().(syscall.WaitStatus).Signal()
	return signal == syscall.SIGTERM || signal == syscall.SIGPIPE
}

func (c *Cmd) TerminateProcessGroup() error {
	if c.getpgidError != nil {
		return errors.WithMessage(c.getpgidError, "Can't terminate process group")
	}

	log.Infof("Sending SIGTERM to process group %d", c.pgid)
	// We ignore errors here because the process group might not exist
	// anymore at this point.
	_ = syscall.Kill(-c.pgid, syscall.SIGTERM) // note the minus sign

	// Close the write ends of any pipes to avoid that Wait blocks
	// until the command has finished printing output (which could be
	// indefinitely).
	c.closeDescriptors(c.CloseAfterWait)

	// Give the process group a few seconds to exit
	select {
	case <-time.After(processGroupTerminationGracePeriod):
		// The process group didn't exit within the grace period, so we
		// send it a SIGKILL now
		log.Infof("Sending SIGKILL to process group %d", c.pgid)
		// We ignore errors here because the process group might not exist
		// anymore at this point.
		_ = syscall.Kill(-c.pgid, syscall.SIGKILL) // note the minus sign
	case <-c.waitDone:
		// The process has already exited, nothing else to do here.
		// Note: This might leave other processes in the process group
		// running (which ignored the SIGTERM).
	}

	return nil
}

func (c *Cmd) prepareProcessGroupTermination() error {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Set PGID so that we're able to terminate the process group on
	// timeout.
	c.SysProcAttr.Setpgid = true

	// To avoid that sending a SIGINT to the controlling terminal's
	// foreground process group does not terminate the child process, we
	// set the foreground process group to the child process group.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	var syscallErr syscall.Errno
	if errors.As(err, &syscallErr) && syscallErr == syscall.ENXIO {
		// There is no controlling terminal
		return nil
	}
	if err != nil {
		return errors.WithStack(err)
	}
	c.SysProcAttr.Ctty = int(tty.Fd())
	c.SysProcAttr.Foreground = true
	return nil
}

func (c *Cmd) getpgid() (int, error) {
	pgid, err := syscall.Getpgid(c.Process.Pid)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return pgid, nil
}
