package daemon

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrNoCmdFound   = errors.New("No cmd found")
	ErrLimitReached = errors.New("Limit reached")
)

type ExitedCmd struct {
	Cmd *exec.Cmd
	Err error
}

// 并发安全
type Limiter struct {
	mu           sync.Mutex
	count, limit int
}

func (l *Limiter) Inc() bool {
	return l.Add(1)
}

func (l *Limiter) Add(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.count+n > l.limit || l.count+n < 0 {
		return false
	}
	l.count += n
	return true
}

func (l *Limiter) Dec() bool {
	return l.Add(-1)
}

func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.count = 0
}

type Daemon struct {
	mu sync.Mutex

	exitedCmdCh    chan *ExitedCmd      // 生产者：runCmd 消费者：reRun
	runningCmds    map[string]*exec.Cmd // 正在运行的cmd
	done           chan struct{}        // 退出daemon和所有child cmd
	restartLimiter *Limiter             // 重启次数限制，4 * len(Cmds) per minute

	Cmds   []*exec.Cmd // Cmds is a list of command to run
	Logger *zap.SugaredLogger
}

func NewDaemon(cmds []*exec.Cmd, logger *zap.SugaredLogger) *Daemon {
	d := &Daemon{
		Cmds:           cmds,
		restartLimiter: &Limiter{limit: 4 * len(cmds)},
		Logger:         logger,
	}
	d.init()
	return d
}

func (d *Daemon) init() {
	d.exitedCmdCh = make(chan *ExitedCmd, 20)
	d.runningCmds = make(map[string]*exec.Cmd, 20)
	d.done = make(chan struct{}, 1)
}

// 主goroutine
func (d *Daemon) Run() {
	// 运行all cmds
	// exitedCmdCh生产者
	go d.run()

	go d.resetLimiter()

	// 接收exitedCmdCh中需要restart的cmd
	// 更改restartLimit
	// exitedCmdCh消费者
	for {
		select {
		// 退出所有goroutine
		case <-d.done:
			d.killAll()
			return
		// 处理exitedCmd
		case c := <-d.exitedCmdCh:
			if c.Err != nil {
				d.Logger.Errorf("cmd: %s exited with err: %s", c.Cmd.String(), c.Err)
				d.Logger.Errorf("cmd: %s will restarting", c.Cmd.String())
			}
			if ok := d.restartLimiter.Inc(); !ok {
				d.Logger.Errorln(c.Cmd.String(), " err:", ErrLimitReached)
				d.exitedCmdCh <- c
				time.Sleep(3 * time.Second)
				continue
			}
			newCmd := exec.Command(c.Cmd.Path, c.Cmd.Args[1:]...)
			go d.startAndWait(newCmd)
		}
	}
}

// run start all cmds and wait for them to exit
func (d *Daemon) run() {

	for _, cmd := range d.Cmds {
		go d.startAndWait(cmd)
	}
	<-d.done
}

func (d *Daemon) Done() chan struct{} {
	return d.done
}

// startAndWait run the cmd and update runningCmds, then wait for it to exit
// startAndWait is producer of exitedCmdCh
func (d *Daemon) startAndWait(cmd *exec.Cmd) {
	var err error
	err = cmd.Start()
	if err != nil {
		err = fmt.Errorf("%s start err: %s", cmd.String(), err)
		exitedCmd := &ExitedCmd{
			Cmd: cmd,
			Err: err,
		}
		d.exitedCmdCh <- exitedCmd
		return
	}

	key := d.hashCmd(cmd)
	d.mu.Lock()
	d.runningCmds[key] = cmd // 更新runningCmds
	d.mu.Unlock()
	err = cmd.Wait()
	if err != nil {
		err = fmt.Errorf("%s exited, err: %s", cmd.String(), err)
	}
	d.mu.Lock()
	delete(d.runningCmds, key) // 更新runningCmds
	d.mu.Unlock()
	exitedCmd := &ExitedCmd{
		Cmd: cmd,
		Err: err,
	}
	d.exitedCmdCh <- exitedCmd
}

// resetLimiter reset restartLimiter every minute
func (d *Daemon) resetLimiter() {
	for {
		select {
		case <-d.done:
			return
		case <-time.After(time.Minute):
			d.restartLimiter.Reset()
		}
	}
}

// hashCmd hash cmd and pid
func (d *Daemon) hashCmd(cmd *exec.Cmd) string {
	hash := fnv.New32()
	cmdstr, pid := cmd.String(), cmd.Process.Pid
	cmdstrpid := cmdstr + strconv.Itoa(pid)
	hash.Write([]byte(cmdstrpid))
	return strconv.Itoa(int(hash.Sum32()))
}

// killAll kill all running cmds
func (d *Daemon) killAll() {
	for _, cmd := range d.runningCmds {
		_ = cmd.Process.Kill()
	}
}

// Close close daemon
func (d *Daemon) Close() {
	close(d.done)
}
