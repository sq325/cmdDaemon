package daemon

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"go.uber.org/zap"
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

	exitedCmdCh chan *daemonCmd
	DCmds       []*daemonCmd

	Logger *zap.SugaredLogger
}

func NewDaemon(ctx context.Context, cmds []*exec.Cmd, logger *zap.SugaredLogger) *Daemon {
	d := &Daemon{
		ctx:         ctx,
		Logger:      logger,
		exitedCmdCh: make(chan *daemonCmd, 20),
	}
	for _, cmd := range cmds {
		dCmd := newDaemonCmd(d.ctx, cmd)
		d.DCmds = append(d.DCmds, dCmd)
	}
	return d
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
				d.Logger.Infoln("Reseted all cmd's limiter")
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
				d.Logger.Infoln("Print all cmd's limiter")
				for _, dCmd := range d.DCmds {
					if dCmd.Status == Exited {
						d.Logger.Errorln("Cmd:", dCmd.Cmd.String(), "exited")
						continue
					}
					dCmd.mu.Lock()
					d.Logger.Infoln(dCmd.Cmd.String(), ". Pid:", dCmd.Cmd.Process.Pid, ". Restarted times:", dCmd.Limiter.count)
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
			d.Logger.Warnf("Cmd %s Err: %v", dcmd.Cmd.String(), dcmd.Err)
			d.Logger.Warnln("Restarting cmd:", dcmd.Cmd.String(), ". Restarted times:", dcmd.Limiter.count)
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
						d.Logger.Warnln("Restarted.")
						dcmd.startAndWait(d.exitedCmdCh)
						return
					}
					// 如果超过limit的次数限制，就不再重启
					d.Logger.Errorln(dcmd.Cmd.String(), " ", ErrLimitReached)
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
	<-d.ctx.Done()
}

// resetLimiter reset all cmds' limiter
func (d *Daemon) resetLimiter() {
	for _, dCmd := range d.DCmds {
		dCmd.Limiter.Reset()
	}
}

// Reload reload all dcmds and ctx
func (d *Daemon) Reload(ctx context.Context, cmds []*exec.Cmd) {
	d.DCmds = make([]*daemonCmd, 0, len(cmds))
	close(d.exitedCmdCh)
	d.exitedCmdCh = make(chan *daemonCmd, 20)
	d.ctx = ctx
	for _, cmd := range cmds {
		dCmd := newDaemonCmd(d.ctx, cmd)
		d.DCmds = append(d.DCmds, dCmd)
	}
}
