package daemon

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
)

// daemonCmd is a cmd that managed by daemon
type daemonCmd struct {
	mu  sync.Mutex
	ctx context.Context

	cmd     *exec.Cmd
	limiter *Limiter // 限制重启次数

	status int   // running: 1, exited: 0
	err    error // 退出原因
}

func newDaemonCmd(ctx context.Context, cmd *exec.Cmd) *daemonCmd {
	return &daemonCmd{
		ctx:     ctx,
		cmd:     cmd,
		limiter: newLimiter(),
	}
}

// update reset the cmd, status and err fields for restarting
func (dcmd *daemonCmd) update() {
	dcmd.mu.Lock()
	defer dcmd.mu.Unlock()

	newCmd := exec.Command(dcmd.cmd.Path, dcmd.cmd.Args[1:]...)
	dcmd.cmd = newCmd
	dcmd.err = nil
}

// startAndWait run the cmd and update runningCmds, then wait for it to exit
// startAndWait is producer of exitedCmdCh
func (dcmd *daemonCmd) startAndWait(ch chan<- *daemonCmd) {
	cmd := dcmd.cmd
	err := cmd.Start()
	if err != nil {
		err = fmt.Errorf("%s start err: %s", cmd.String(), err)
		dcmd.err = err
		ch <- dcmd
		return
	}
	dcmd.status = _running

	err = cmd.Wait()
	if err != nil {
		err := fmt.Errorf("cmd: %s exited with err: %s, exitCode: %d", dcmd.cmd.String(), dcmd.err, cmd.ProcessState.ExitCode())
		dcmd.err = err
	}
	dcmd.status = _exited
	ch <- dcmd
}
