package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/sq325/cmdDaemon/daemon"
	"github.com/sq325/cmdDaemon/internal/tool"

	"github.com/gin-gonic/gin"
)

type SvcManager interface {
	Restart() error // restart daemon process and child processes
	Reload() error  // reload child processes
	List() []byte   // list all port and cmd
	Update() error  // update config file
	Stop() error    // stop daemon process
	Health() bool   // check health
}

// Handler implement SvcManager interface
// Handler 处理reload和restart请求
type Handler struct {
	logger *slog.Logger
	Daemon *daemon.Daemon
}

var _ SvcManager = (*Handler)(nil)

// NewHandler create a new Handler
func NewSvcManager(logger *slog.Logger, d *daemon.Daemon) SvcManager {
	return &Handler{
		logger: logger,
		Daemon: d,
	}
}

func (h *Handler) Restart() error {
	return restart()
}

func (h *Handler) Reload() error {

	if len(h.Daemon.DCmds) == 0 {
		return errors.New("no child processes")
	}

	var errs error
	for _, dcmd := range h.Daemon.DCmds {
		if dcmd.Status == daemon.Exited {
			continue
		}
		pid := dcmd.Cmd.Process.Pid
		err := syscall.Kill(pid, syscall.SIGHUP) // send HUP sig
		if err != nil {
			errtmp := fmt.Errorf("cmd: %s Pid: %d kill failed. %v", dcmd.Cmd.String(), pid, err)
			errs = errors.Join(errs, errtmp)
		}
	}
	return errs
}

func (h *Handler) List() []byte {
	addrCmd, err := addrCmdMap(h.Daemon.DCmds)
	if err != nil {
		h.logger.Error("AddrCmdMap error", "error", err)
		return nil
	}
	if addrCmd == nil {
		return nil
	}

	var bys []byte
	for addr, cmd := range addrCmd {
		var port string
		url, err := url.Parse("http://" + addr)
		if err != nil {
			h.logger.Error("Parse addr error", "addr", addr, "error", err)
			port, err = portFromAddr(addr)
			if err != nil {
				h.logger.Error("portFromAddr error", "error", err)
				bys = append(bys, []byte(addr+" "+cmd+"\n")...)
				continue
			}
			bys = append(bys, []byte(port+" "+cmd+"\n")...)
		}
		bys = append(bys, []byte(url.Port()+" "+cmd+"\n")...)
	}

	return bys
}

func (h *Handler) Update() error {
	err := GitPull()
	if err != nil {
		h.logger.Error("git pull error", "error", err)
	} else {
		h.logger.Info("git pull success")
	}
	return err
}

func (h *Handler) Stop() error {
	pid := os.Getpid()
	return syscall.Kill(pid, syscall.SIGTERM)
}

func (h *Handler) Health() bool {
	return true
}

// ListPortAndCmd list all cmd and listen port
func (h *Handler) ListPortAndCmd(c *gin.Context) {
	addrCmd, err := addrCmdMap(h.Daemon.DCmds)
	if err != nil {
		h.logger.Error("AddrCmdMap error", "error", err)
		c.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	if addrCmd == nil {
		c.Writer.WriteHeader(http.StatusNoContent)
		return
	}

	for addr, cmd := range addrCmd {
		var port string
		url, err := url.Parse("http://" + addr)
		if err != nil {
			h.logger.Error("Parse addr error", "addr", addr, "error", err)
			port, err = portFromAddr(addr)
			if err != nil {
				h.logger.Error("portFromAddr error", "error", err)
				c.Writer.Write([]byte(addr + " " + cmd + "\n"))
				continue
			}
			c.Writer.Write([]byte(port + " " + cmd + "\n"))
		}
		c.Writer.Write([]byte(url.Port() + " " + cmd + "\n"))
	}
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Write([]byte("--------------------\n"))
	c.Writer.Write([]byte("All " + strconv.Itoa(len(h.Daemon.DCmds)) + ", List " + strconv.Itoa(len(addrCmd))))
}

func (h *Handler) UpdateConfig(c *gin.Context) {
	err := GitPull()
	if err != nil {
		h.logger.Error("git pull error", "error", err)
		c.Writer.Write([]byte("git pull err"))
	} else {
		h.logger.Info("git pull success")
		c.Writer.Write([]byte("git pull success"))
	}
}

// Restart sends a SIGHUP to the daemon process
func restart() error {
	pid := os.Getpid()
	return syscall.Kill(pid, syscall.SIGHUP)
}

// Generate consul service config
func (h *Handler) ConsulSvc(c *gin.Context) {
	dcmds := h.Daemon.DCmds
	if len(dcmds) == 0 {
		c.Writer.WriteHeader(http.StatusNoContent)
		c.Writer.Write([]byte("No services"))
		return
	}
}

// GitPull git pull origin master without SSH_ASKPASS
func GitPull() error {
	checkoutCmd := exec.Command("git", "checkout", "master")
	cmd := exec.Command("git", "pull", "origin", "master")

	// git checkout
	err := checkoutCmd.Run()
	if err != nil {
		return fmt.Errorf("git checkout err: %v", err)
	}

	// delete SSH_ASKPASS
	cmdEnv := os.Environ()
	for i, env := range cmdEnv {
		if strings.Contains(env, "SSH_ASKPASS") {
			cmdEnv = append(cmdEnv[:i], cmdEnv[i+1:]...)
		}
	}
	cmd.Env = cmdEnv

	err = cmd.Run()
	return err
}

func portFromAddr(addr string) (string, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("SplitHostPort err: %w", err)
	}
	return port, nil
}

func cors(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")                                                            // 允许访问所有域，可以换成具体url，注意仅具体url才能带cookie信息
		w.Header().Add("Access-Control-Allow-Headers", "Content-Type,AccessToken,X-CSRF-Token, Authorization, Token") //header的类型
		w.Header().Add("Access-Control-Allow-Credentials", "true")                                                    //设置为true，允许ajax异步请求带cookie信息
		w.Header().Add("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")                             //允许请求方法
		w.Header().Set("content-type", "application/json;charset=UTF-8")                                              //返回数据格式是json
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		f(w, r)
	}
}

func addrCmdMap(dcmds []*daemon.DaemonCmd) (map[string]string, error) {
	if len(dcmds) == 0 {
		return nil, nil
	}
	var addrCmd = make(map[string]string, len(dcmds))
	pidAddr, err := tool.PidAddr()
	if err != nil {
		return nil, fmt.Errorf("pidAddr err: %w", err)
	}

	for _, dcmd := range dcmds {
		if dcmd.Cmd == nil || dcmd.Cmd.Process == nil {
			continue
		}
		pid := strconv.Itoa(dcmd.Cmd.Process.Pid)

		if addr, ok := pidAddr[pid]; ok {
			addrCmd[addr] = dcmd.Cmd.String()
		}
	}

	return addrCmd, nil
}
