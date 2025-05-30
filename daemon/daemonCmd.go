package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/sq325/cmdDaemon/config"
	"github.com/sq325/cmdDaemon/internal/tool"
)

// Default Annotations keys for DaemonCmd
const (
	AnnotationsNameKey        = config.AnnotationsNameKey        // 服务名称
	AnnotationsIPKey          = config.AnnotationsIPKey          // 管理IP
	AnnotationsPortKey        = config.AnnotationsPortKey        // 端口号
	AnnotationsMetricsPathKey = config.AnnotationsMetricsPathKey // metrics路径
	AnnotationsHostnameKey    = config.AnnotationsHostnameKey    // 主机名
	AnnotationsAppKey         = config.AnnotationsAppKey         // 应用名称, 用于区分不同应用的cmd
)

// DaemonCmd is a cmd that managed by daemon
type DaemonCmd struct {
	mu  sync.Mutex
	ctx context.Context

	Cmd     *exec.Cmd
	Limiter *Limiter // 限制重启次数

	// Annotations:
	// 	 name: "prometheus" // 默认basename cmd.Args[0]
	// 	 port: "9091" // 需要人工填写
	// 	 hostname: "proxy-a" // 默认os.Hostname()
	// 	 ip: "12.12.12.12" // 默认/etc/hosts中根据hostname查找
	// 	 metricsPath: "/metrics" // 需填写，如果为""，表示该cmd不提供metrics
	Annotations map[string]string // cmd的注释信息, name, hostName, ip, port
	Status      int               // running: 1, exited: 0
	Err         error             // 退出原因

	logDir string // 日志文件路径
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

	// log
	if dcmd.logDir != "" {
		// 确保日志目录存在
		if err := os.MkdirAll(dcmd.logDir, 0755); err != nil {
			dcmd.Err = fmt.Errorf("create log dir %s err: %v", dcmd.logDir, err)
			return
		}

		logfilePath := filepath.Join(dcmd.logDir, fmt.Sprintf("%s_%s_%s.log", dcmd.Annotations[AnnotationsNameKey],
			dcmd.Annotations[AnnotationsPortKey], dcmd.CmdHash()))

		// 以追加模式打开日志文件，如果不存在则创建
		f, err := os.OpenFile(logfilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			dcmd.Err = fmt.Errorf("open log file %s err: %v", logfilePath, err)
			return
		}
		// 确保在函数结束时关闭日志文件
		defer func() {
			if f != nil {
				f.Close()
			}
		}()

		cmd.Stdout = f
		cmd.Stderr = f
	}

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

// CmdHash return a hash of the cmd
// the hash is computed by the name and args of the cmd
// args are sorted
func (dcmd *DaemonCmd) CmdHash() string {
	return tool.HashCmd(dcmd.Cmd)
}

type dcmdFunc func(dcmd *DaemonCmd)

func withLogDir(logDir string) dcmdFunc {
	return func(dcmd *DaemonCmd) {
		if dcmd == nil {
			return
		}
		dcmd.logDir = logDir
	}
}
