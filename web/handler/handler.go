package handler

import (
	"cmdDaemon/daemon"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"go.uber.org/zap"
)

// Handler 处理reload和restart请求
type Handler struct {
	logger *zap.SugaredLogger
	Daemon *daemon.Daemon
}

// NewHandler create a new Handler
func NewHandler(logger *zap.SugaredLogger, d *daemon.Daemon) *Handler {
	return &Handler{
		logger: logger,
		Daemon: d,
	}
}

// RegisterHandleFunc register all http hanler funcs
func (h *Handler) RegisterHandleFunc() {
	http.HandleFunc("/reload", h.Reload)        // reload child processes
	http.HandleFunc("/restart", h.ReloadDaemon) // reload daemon process and child processes
	http.HandleFunc("/list", h.ListPortAndCmd)  // list all port and cmd
	http.HandleFunc("/update", h.UpdateConfig)  // update config file
	http.HandleFunc("/stop", h.Stop)            // stop daemon process
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
	restart()
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ReloadDaemon Success"))
}

func (h *Handler) Restart(w http.ResponseWriter, req *http.Request) {
	// if has update, git pull
	if err := req.ParseForm(); err != nil {
		h.logger.Error("ParseForm err:", err)
	}
	if hasUpdate := req.Form.Has("update"); hasUpdate {
		err := GitPull()
		if err != nil {
			h.logger.Error("git pull err:", err)
			w.Write([]byte("git pull err\n"))
		} else {
			h.logger.Info("git pull success")
			w.Write([]byte("git pull success\n"))
		}
	}

	restart()
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Restart Success"))
}

// Reload send SIGHUP signal to all cmds processes except daemon process
// /reload
func (h *Handler) Reload(w http.ResponseWriter, req *http.Request) {
	if len(h.Daemon.DCmds) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// if has update, git pull
	if err := req.ParseForm(); err != nil {
		h.logger.Error("ParseForm err:", err)
	}
	if hasUpdate := req.Form.Has("update"); hasUpdate {
		err := GitPull()
		if err != nil {
			h.logger.Error("git pull err:", err)
			w.Write([]byte("git pull err\n"))

		} else {
			h.logger.Info("git pull success")
			w.Write([]byte("git pull success\n"))
		}
	}

	for _, dcmd := range h.Daemon.DCmds {
		if dcmd.Status == daemon.Exited {
			continue
		}
		pid := dcmd.Cmd.Process.Pid
		syscall.Kill(pid, syscall.SIGHUP)
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Reload Success"))
}

// ListPortAndCmd list all cmd and listen port
func (h *Handler) ListPortAndCmd(w http.ResponseWriter, req *http.Request) {
	portCmd, err := PortCmdMap(h.Daemon.DCmds)
	if err != nil {
		h.logger.Error("PortCmdMap err:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if portCmd == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	for addr, cmd := range portCmd {
		port := addr
		url, err := url.Parse("http://" + addr)
		if err != nil {
			h.logger.Error("Parse addr ", addr, " err:", err)
		} else {
			port = url.Port()
		}
		w.Write([]byte(port + " " + cmd + "\n"))
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("List Done"))
}

func (h *Handler) UpdateConfig(w http.ResponseWriter, req *http.Request) {
	err := GitPull()
	if err != nil {
		h.logger.Error("git pull err:", err)
		w.Write([]byte("git pull err"))
	} else {
		h.logger.Info("git pull success")
		w.Write([]byte("git pull success"))
	}
}

func (h *Handler) Stop(w http.ResponseWriter, req *http.Request) {
	pid := os.Getpid()
	syscall.Kill(pid, syscall.SIGTERM)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Stop Success"))
}

// Restart sends a SIGHUP to the daemon process
func restart() {
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

// PortCmdMap return a map of port and cmd
func PortCmdMap(dcmds []*daemon.DaemonCmd) (map[string]string, error) {
	if len(dcmds) == 0 {
		return nil, nil
	}
	var portCmd = make(map[string]string, len(dcmds))
	var spacePattern = regexp.MustCompile(`\s+`)

	// generate portCmd
	for _, dcmd := range dcmds {
		pid := strconv.Itoa(dcmd.Cmd.Process.Pid)
		pidListen, err := exec.Command("sh", "-c", "lsof -Pp "+pid+" | grep LISTEN").Output()
		if err != nil {
			return nil, fmt.Errorf("lsof err: %v", err)
		}
		pidListenS := strings.Split(string(pidListen), "\n")

		port := "null"
		for _, line := range pidListenS {
			if strings.Contains(line, "LISTEN") {
				lineSlice := spacePattern.Split(line, -1)
				if len(lineSlice) < 2 {
					continue
				}
				port = lineSlice[len(lineSlice)-2]
				break
			}
		}
		portCmd[port] = dcmd.Cmd.String()
	}

	return portCmd, nil
}
