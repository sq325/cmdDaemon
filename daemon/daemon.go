package daemon

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"time"

	"github.com/sq325/cmdDaemon/internal/tool"
)

const (
	Exited = iota
	Running
)

var (
	ErrNoCmdFound   = errors.New("no cmd found")
	ErrLimitReached = errors.New("restart limit reached")
)

// Daemon is a daemon that manages multiple dcmds
type Daemon struct {
	ctx context.Context

	exitedCmdCh chan *DaemonCmd
	DCmds       []*DaemonCmd

	Logger *slog.Logger
}

func NewDaemon(ctx context.Context, dcmds []*DaemonCmd, logger *slog.Logger) *Daemon {

	return &Daemon{
		ctx:         ctx,
		exitedCmdCh: make(chan *DaemonCmd, 20),
		DCmds:       dcmds,
		Logger:      logger,
	}
}

// 主goroutine
func (d *Daemon) Run() {
	// 运行all cmds
	// exitedCmdCh生产者
	go d.run()

	// 每30分钟重置所有cmd的limiter
	limitResetTicker := time.NewTicker(30 * time.Minute)
	defer limitResetTicker.Stop()
	go func() {
		for {
			select {
			case <-d.ctx.Done():
				return
			case <-limitResetTicker.C:
				d.resetLimiter()
				d.Logger.Info("Reseted all cmd's limiter")
			}
		}
	}()

	// 每15分钟打印一次所有running cmd
	printCmdTicker := time.NewTicker(20 * time.Minute)
	defer printCmdTicker.Stop()
	go func() {
		for {
			select {
			case <-d.ctx.Done():
				return
			case <-printCmdTicker.C:
				d.Logger.Info("Print all cmd's limiter")
				for _, dCmd := range d.DCmds {
					if dCmd.Status == Exited {
						d.Logger.Error("Command exited", "cmd", dCmd.Cmd.String())
						continue
					}
					dCmd.mu.Lock()
					d.Logger.Info("Command status", "cmd", dCmd.Cmd.String(), "pid", dCmd.Cmd.Process.Pid, "restarts", dCmd.Limiter.count)
					dCmd.mu.Unlock()
				}
				printCmdTicker.Reset(15 * time.Minute)
			}
		}
	}()

	// 接收exitedCmdCh中需要restart的cmd
	// 更改restartLimit
	// exitedCmdCh消费者
	for {
		select {
		// 退出所有goroutine
		case <-d.ctx.Done():
			return
		// 处理exitedCmd
		case dcmd := <-d.exitedCmdCh:
			// 打印错误原因
			dcmd.mu.Lock()
			d.Logger.Warn("Command error", "cmd", dcmd.Cmd.String(), "error", dcmd.Err)
			d.Logger.Warn("Restarting command", "cmd", dcmd.Cmd.String(), "restarts", dcmd.Limiter.count)
			dcmd.mu.Unlock()
			go func() {
				select {
				case <-d.ctx.Done():
					return
				// 等到下次重启时间到了再重启
				case <-time.After(time.Until(dcmd.Limiter.next())):
					// 重启cmd
					dcmd.update()
					// 没超过limit，重启cmd
					if ok := dcmd.Limiter.Inc(); ok {
						d.Logger.Warn("Command restarted")
						dcmd.startAndWait(d.exitedCmdCh)
						return
					}
					// 如果超过limit的次数限制，就不再重启
					d.Logger.Error("Command restart limit reached", "cmd", dcmd.Cmd.String(), "error", ErrLimitReached.Error())
					return
				}
			}()
		}
	}
}

// run start all cmds and wait for them to exit
func (d *Daemon) run() {
	for _, dCmd := range d.DCmds {
		go dCmd.startAndWait(d.exitedCmdCh)
	}
}

// resetLimiter reset all cmds' limiter
func (d *Daemon) resetLimiter() {
	for _, dCmd := range d.DCmds {
		dCmd.Limiter.Reset()
	}
}

// Reload reload all dcmds and ctx
func (d *Daemon) Reload(ctx context.Context, cmds []*exec.Cmd, annotationsList []map[string]string) {
	d.DCmds = make([]*DaemonCmd, 0, len(cmds))
	// drain channel
	close(d.exitedCmdCh)
	if d.exitedCmdCh != nil {
		for range d.exitedCmdCh {
		}
	}

	d.exitedCmdCh = make(chan *DaemonCmd, 20)
	d.ctx = ctx
	for i, cmd := range cmds {
		dCmd := NewDaemonCmd(d.ctx, cmd, annotationsList[i])
		d.DCmds = append(d.DCmds, dCmd)
	}
}

// GetDCmds return all dcmds
func (d *Daemon) GetDCmds() []*DaemonCmd {
	return d.DCmds
}

// cmdsLen return the number of cmds
func (d *Daemon) cmdsLen() int {
	return len(d.DCmds)
}

// GetExitedCmdLen return the number of exited cmds
func (d *Daemon) GetExitedCmdLen() int {
	var count int
	for _, dcmd := range d.DCmds {
		if dcmd.Status == Exited {
			count++
		}
	}
	return count
}

func (d *Daemon) GetRunningCmdLen() int {
	var count int
	for _, dcmd := range d.DCmds {
		if dcmd.Status == Running {
			count++
		}
	}
	return count
}

func (d *Daemon) GetDCmdByCmd(cmd *exec.Cmd) (*DaemonCmd, error) {
	for _, dcmd := range d.DCmds {
		hash := tool.HashCmd(cmd)
		if hash == dcmd.CmdHash() {
			return dcmd, nil
		}
	}
	return nil, ErrNoCmdFound
}
