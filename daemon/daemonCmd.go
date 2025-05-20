package daemon

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
)

// DaemonCmd is a cmd that managed by daemon
type DaemonCmd struct {
	mu  sync.Mutex
	ctx context.Context

	Cmd     *exec.Cmd
	Limiter *Limiter // 限制重启次数

	Annotations map[string]string // cmd的注释信息
	Status      int               // running: 1, exited: 0
	Err         error             // 退出原因
}

func NewDaemonCmd(ctx context.Context, cmd *exec.Cmd, anotations map[string]string) *DaemonCmd {
	return &DaemonCmd{
		ctx:         ctx,
		Cmd:         cmd,
		Annotations: anotations,
		Limiter:     NewLimiter(),
	}
}

// update reset the cmd, status and err fields for restarting
func (dcmd *DaemonCmd) update() {
	dcmd.mu.Lock()
	defer dcmd.mu.Unlock()

	newCmd := exec.Command(dcmd.Cmd.Path, dcmd.Cmd.Args[1:]...)
	dcmd.Cmd = newCmd
	dcmd.Err = nil
}

// startAndWait run the cmd and update runningCmds, then wait for it to exit
// startAndWait is producer of exitedCmdCh
func (dcmd *DaemonCmd) startAndWait(ch chan<- *DaemonCmd) {
	cmd := dcmd.Cmd
	err := cmd.Start()
	if err != nil {
		err = fmt.Errorf("%s start err: %v", cmd.String(), err)
		dcmd.Err = err
		select {
		case <-dcmd.ctx.Done():
			return
		default:
			ch <- dcmd
		}
		return
	}
	dcmd.Status = Running

	err = cmd.Wait()
	if err != nil {
		err := fmt.Errorf("cmd: %s exited with err: %v, exitCode: %d", dcmd.Cmd.String(), dcmd.Err, cmd.ProcessState.ExitCode())
		dcmd.Err = err
	}
	dcmd.Status = Exited
	// 防止ch已经close，send导致panic
	select {
	case <-dcmd.ctx.Done(): // cancel
		return
	default:
		ch <- dcmd
	}
}
