package daemon

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"go.uber.org/zap"
)

const (
	_exited = iota
	_running
)

var (
	ErrNoCmdFound   = errors.New("no cmd found")
	ErrLimitReached = errors.New("restart limit reached")
)

// Daemon is a daemon that manages multiple dcmds
type Daemon struct {
	ctx context.Context

	exitedCmdCh chan *daemonCmd
	dCmds       []*daemonCmd

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
		d.dCmds = append(d.dCmds, dCmd)
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
	printCmdTicker := time.NewTicker(15 * time.Minute)
	defer printCmdTicker.Stop()
	go func() {
		for {
			select {
			case <-d.ctx.Done():
				return
			case <-printCmdTicker.C:
				d.Logger.Infoln("Print all cmd's limiter")
				for _, dCmd := range d.dCmds {
					if dCmd.status == _exited {
						continue
					}
					dCmd.mu.Lock()
					d.Logger.Infoln(dCmd.cmd.String(), " ", dCmd.limiter.count)
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
			d.Logger.Warnln("Err:", dcmd.err)
			d.Logger.Warnln("Restarting cmd: ", dcmd.cmd.String(), ". Restarted times: ", dcmd.limiter.count)
			dcmd.mu.Unlock()
			go func() {
				select {
				case <-d.ctx.Done():
					return
				// 等到下次重启时间到了再重启
				case <-time.After(time.Until(dcmd.limiter.next())):
					// 重启cmd
					dcmd.update()
					// 没超过limit，重启cmd
					if ok := dcmd.limiter.Inc(); ok {
						d.Logger.Warnln("Restarted!")
						dcmd.startAndWait(d.exitedCmdCh)
						return
					}
					// 如果超过limit的次数限制，就不再重启
					d.Logger.Errorln(dcmd.cmd.String(), " ", ErrLimitReached)
					return
				}
			}()
		}
	}
}

// run start all cmds and wait for them to exit
func (d *Daemon) run() {
	for _, dCmd := range d.dCmds {
		go dCmd.startAndWait(d.exitedCmdCh)
	}
	<-d.ctx.Done()
}

func (d *Daemon) resetLimiter() {
	for _, dCmd := range d.dCmds {
		dCmd.limiter.Reset()
	}
}
