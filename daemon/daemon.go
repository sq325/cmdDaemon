package daemon

import (
	"os"
	"os/exec"
)

type Daemon struct {
	signCh      chan os.Signal
	exitedCmdCh chan *exec.Cmd
	runningCmds map[string]*exec.Cmd
	done        chan struct{}
	// Cmds is a list of command to run
	Cmds []*exec.Cmd
}

func NewDaemon(cmds []*exec.Cmd) *Daemon {
	return &Daemon{
		Cmds: cmds,
	}
}
func (d *Daemon) Run() error {
	d.run()
	return nil
}

func (d *Daemon) run() {
	// 等待进程退出
	go func() {
		for {
			select {
			case <-d.done:
				return
			case c := <-d.exitedCmdCh:
				d.reRun(c)
			}
		}
	}()
	// 等待信号
	go func() {
		for {
			select {
			case <-d.done:
				return
			case <-d.signCh:
				d.Stop()
			}
		}
	}()

	for _, c := range d.Cmds {
		d.runningCmds[c.Path] = c
		c.Start()
	}
}
func (d *Daemon) reRun(c *exec.Cmd) {
	if _, ok := d.runningCmds[c.Path]; ok {
		delete(d.runningCmds, c.Path)
	}
	c.Start()
	d.runningCmds[c.Path] = c
}

func (d *Daemon) Stop() {
	d.done <- struct{}{}
	for _, c := range d.Cmds {
		c.Process.Signal(os.Interrupt)
	}
}
