package handler

import (
	"cmdDaemon/daemon"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// Handler 处理reload和restart请求
type Handler struct {
	logger *zap.SugaredLogger
	Daemon *daemon.Daemon
}

func NewHandler(logger *zap.SugaredLogger, d *daemon.Daemon) *Handler {
	return &Handler{
		logger: logger,
		Daemon: d,
	}
}

// RegisterHandleFunc register all http hanler funcs
func (h *Handler) RegisterHandleFunc() {
	http.HandleFunc("/reload", h.Reload)
	http.HandleFunc("/restart", h.ReloadDaemon)
}

// Listen start register handleFuncs and start a http server
func (h *Handler) Listen(port string) {
	h.RegisterHandleFunc()
	h.logger.Error(http.ListenAndServe(":"+port, nil))
}

// ReloadDaemon git pull origin master and restart daemon process
// /restart
func (h *Handler) ReloadDaemon(w http.ResponseWriter, req *http.Request) {
	err := GitPull()
	if err != nil {
		h.logger.Error("git pull err:", err)
	} else {
		h.logger.Info("git pull success")
	}
	Restart()
}

// Reload send SIGHUP signal to all cmds processes except daemon process
// /reload
func (h *Handler) Reload(w http.ResponseWriter, req *http.Request) {
	if len(h.Daemon.DCmds) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	for _, dcmd := range h.Daemon.DCmds {
		if dcmd.Status == daemon.Exited {
			continue
		}
		pid := dcmd.Cmd.Process.Pid
		syscall.Kill(pid, syscall.SIGHUP)
		time.Sleep(time.Second)
	}
	w.WriteHeader(http.StatusOK)
}

// Restart sends a SIGHUP to the daemon process
func Restart() {
	pid := os.Getpid()
	syscall.Kill(pid, syscall.SIGHUP)
}

// GitPull git pull origin master without SSH_ASKPASS
func GitPull() error {
	cmd := exec.Command("git", "pull", "origin", "master")

	// delete SSH_ASKPASS
	cmdEnv := os.Environ()
	for i, env := range cmdEnv {
		if strings.Contains(env, "SSH_ASKPASS") {
			cmdEnv = append(cmdEnv[:i], cmdEnv[i+1:]...)
		}
	}
	cmd.Env = cmdEnv

	err := cmd.Run()
	return err
}

func CmdsAndPorts() {
	cmd := exec.Command("ps", "aux")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}
