package main

import (
	"cmdDaemon/config"
	"cmdDaemon/daemon"
	"cmdDaemon/register"
	"cmdDaemon/web/handler"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "cmdDaemon/docs"

	"context"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	fork "github.com/sevlyar/go-daemon"
	"github.com/spf13/pflag"

	swaggerFiles "github.com/swaggo/files"

	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	complementConsul "github.com/sq325/kitComplement/consul"
	tool "github.com/sq325/kitComplement/tool"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	daemontool "cmdDaemon/internal/tool"
	dmetrics "cmdDaemon/web/metrics"

	"github.com/sq325/kitComplement/instrumentation"
)

const (
	_service     = "daemon"
	_version     = "v0.5.6"
	_versionInfo = "bugfix"
)

// flags
var (
	createConfFile *bool     = pflag.Bool("config.createDefault", false, "Generate a default config file.")
	configFile     *string   = pflag.String("config.file", "./daemon.yml", "Daemon configuration file name.")
	version        *bool     = pflag.BoolP("version", "v", false, "Print version information.")
	svcIP          *string   = pflag.String("svcIP", "", "svc ip, default hostAdmIp")
	port           *string   = pflag.String("web.port", "9090", "Port to listen.")
	consulAddr     *string   = pflag.String("consul.addr", "", "Consul address. e.g. localhost:8500")
	registerCmds   *bool     = pflag.Bool("consul.regChild", false, "Register all child processes to consul.")
	consulIfList   *[]string = pflag.StringSlice("consul.infList", []string{"bond0", "eth0", "eth1"}, `Network interface list. e.g. --consul.infList="v1,v2"`)
	// consulSvcRegFile *string   = pflag.String("consul.svcRegFile", "./services.json", "Consul service register file name.")
	logLevel *string = pflag.String("log.level", "info", "Log level. e.g. debug, info, warn, error, dpanic, panic, fatal")

	printCmds *bool = pflag.BoolP("printCmds", "p", false, "Print cmds parse from config.")
	killCmds  *bool = pflag.Bool("killCmds", false, "Kill all child processes from config.")
	// printConsulConf *bool = pflag.Bool("printConsulConf", false, "Print consul config.")
)

var (
	buildTime      string
	buildGoVersion string
	author         string
)

var (
	conf *config.Conf

	signCh = make(chan os.Signal)
)

func init() {
	pflag.Parse()
}

// @title			守护进程服务
// @version		0.5.6

// @license.name	Apache 2.0
func main() {
	if *createConfFile {
		createConfigFile()
		return
	}
	if *version {
		fmt.Println(_service, _version)
		fmt.Println("build time:", buildTime)
		fmt.Println("go version:", buildGoVersion)
		fmt.Println("author:", author)
		return
	}

	// config init
	initConf()
	if *printCmds {
		cmds := createCmds(conf)
		if len(cmds) == 0 {
			fmt.Println("No cmd to print.")
			return
		}
		for _, cmd := range cmds {
			fmt.Println(cmd.String())
		}
		return
	}

	// Clear potential zombie processes based on the configuration file
	// only to be used when the daemon panics.
	if *killCmds {
		cmds := createCmds(conf)
		if len(cmds) == 0 {
			fmt.Println("No cmd to kill.")
			return
		}
		for _, cmd := range cmds {
			err := killcmd(cmd)
			if err != nil {
				fmt.Println(err)
			}
		}
		return
	}

	cntxt := newForkCtx()
	child, err := cntxt.Reborn()
	if err != nil {
		log.Fatal("Unable to run: ", err)
	}
	if child != nil { // 如果此时在父进程中，直接退出
		return
	}
	defer cntxt.Release() // 在函数结束时重启stdin、stdout、stderr

	fmt.Println("- - - - - - - - - - - - - - -")
	fmt.Printf("Daemon started %s\n", time.Now().Format(time.DateTime))

	// 初始化日志
	var logger *zap.SugaredLogger
	switch *logLevel {
	case "debug":
		logger = createLogger(zapcore.DebugLevel)
	case "info":
		logger = createLogger(zapcore.InfoLevel)
	case "warn":
		logger = createLogger(zapcore.WarnLevel)
	case "error":
		logger = createLogger(zapcore.ErrorLevel)
	default:
		logger = createLogger(zapcore.InfoLevel)
	}
	logger.Debugln("port: ", *port)
	logger.Debugln("consulAddr: ", *consulAddr)

	// config
	cmds := createCmds(conf)
	if len(cmds) == 0 {
		logger.Fatalln("No cmd to run. Daemon existed.")
		return
	}

	// signal
	signal.Notify(signCh, syscall.SIGHUP, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	// 初始化Daemon
	onceDaemon := sync.OnceValue(func() *daemon.Daemon {
		return createDaemon(ctx, cmds, logger)
	})
	d := onceDaemon()
	logger.Infoln("Daemon created.")
	logger.Debugf("daemon: %+v\n", d.DCmds)
	go d.Run()                  // run cmds
	time.Sleep(5 * time.Second) // wait for cmds running

	// 初始化svcManager
	svc := handler.NewSvcManager(logger, d)

	metrics := instrumentation.NewMetrics()
	instrumentingMiddleware := instrumentation.InstrumentingMiddleware(metrics)
	daemonMetrics := dmetrics.NewDaemonMetrics(d)
	prometheus.Unregister(promcollectors.NewGoCollector())
	healthSvc := func() bool {
		return svc.Health()
	}

	// 注册路由
	mux := gin.Default()
	gin.SetMode(gin.ReleaseMode)

	mux.PUT("/restart",
		instrumentation.GinHandlerFunc(
			"PUT",
			"/restart",
			instrumentingMiddleware(
				handler.MakeRestartEndpoint(svc),
			),
			handler.DecodeRequest,
			handler.EncodeResponse,
		),
	)

	mux.PUT("/reload",
		instrumentation.GinHandlerFunc(
			"PUT",
			"/reload",
			instrumentingMiddleware(
				handler.MakeReloadEndpoint(svc),
			),
			handler.DecodeRequest,
			handler.EncodeResponse,
		),
	)

	mux.GET("/list",
		instrumentation.GinHandlerFunc(
			"GET",
			"/list",
			instrumentingMiddleware(
				handler.MakeListEndpoint(svc),
			),
			handler.DecodeRequest,
			handler.EncodeResponse,
		),
	)

	mux.PUT("/update",
		instrumentation.GinHandlerFunc(
			"PUT",
			"/update",
			instrumentingMiddleware(
				handler.MakeUpdateEndpoint(svc),
			),
			handler.DecodeRequest,
			handler.EncodeResponse,
		),
	)

	mux.PUT("/stop",
		instrumentation.GinHandlerFunc(
			"PUT",
			"/stop",
			instrumentingMiddleware(
				handler.MakeStopEndpoint(svc),
			),
			handler.DecodeRequest,
			handler.EncodeResponse,
		),
	)

	mux.Any("/health",
		func(c *gin.Context) {
			if healthSvc() {
				c.String(200, "%s service is healthy", _service)
				return
			}
			c.String(500, "%s service is unhealthy", _service)
		},
	)
	mux.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	mux.GET("/metrics", func(c *gin.Context) {
		daemonMetrics.CmdStatusTotal.With("status", "running").Set(float64(d.GetRunningCmdLen()))
		daemonMetrics.CmdStatusTotal.With("status", "exited").Set(float64(d.GetExitedCmdLen()))
		promhttp.Handler().ServeHTTP(c.Writer, c.Request)
	})

	// register service
	var r complementConsul.Registrar
	var node *register.Node
	if *consulAddr != "" {
		{
			c := strings.Split(strings.TrimSpace(*consulAddr), ":")
			ip := c[0]
			p, _ := strconv.Atoi(c[1])
			consulClient := complementConsul.NewConsulClient(ip, p)
			logger := complementConsul.NewLogger()
			r = complementConsul.NewRegistrar(consulClient, logger)
		}

		// register node
		node, err := register.NewNode(*consulIfList)
		if err != nil {
			logger.Errorf("NewNode err: %v", err)
		}
		err = node.Register(*consulAddr)
		if err != nil {
			logger.Errorf("Node Register err: %v", err)
		}

		// register daemon svc
		var svc *complementConsul.Service
		{
			if *svcIP == "" {
				*svcIP, err = tool.HostAdmIp(*consulIfList)
				if err != nil {
					logger.Errorf("HostAdmIp err: %v; intfList: %+v", err, *consulIfList)
				}
			}
			port, _ := strconv.Atoi(*port)
			svc = &complementConsul.Service{
				Name: _service,
				ID:   _service + "_" + node.Name,
				IP:   *svcIP,
				Port: port,
				Check: struct {
					Path     string
					Interval string
					Timeout  string
				}{
					Path:     "/health",
					Interval: "60s",
					Timeout:  "10s",
				},
			}
		}
		r.Register(svc)
		logger.Info("Daemon registered.")
		defer r.Deregister(svc)

	}

	// register cmds
	registerCmdsF := func() {
		if *consulAddr != "" && *registerCmds && len(d.DCmds) > 0 {
			pidAddrM, _ := daemontool.PidAddr()
			pattern := regexp.MustCompile(`prometheus_(?P<tag>[\w\.]+?)\.ya?ml`)
			for _, dcmd := range d.DCmds {
				if dcmd.Status == daemon.Exited {
					continue
				}

				path := dcmd.Cmd.Path
				cmdStr := dcmd.Cmd.String()
				pid := strconv.Itoa(dcmd.Cmd.Process.Pid)
				addr, ok := pidAddrM[pid]
				if !ok {
					logger.Errorf("pid not found: %s, name: %s", pid, path)
					continue
				}
				port, err := strconv.Atoi(daemontool.Parseport(addr))
				if err != nil {
					logger.Errorf("Parseport addr %s err: %v", err, addr)
					continue
				}
				var (
					name      string
					tags      []string
					checkPath string
				)

				if strings.Contains(path, "prometheus") {
					tagIndex := pattern.SubexpIndex("tag")
					matches := pattern.FindStringSubmatch(cmdStr)
					if len(matches) > 0 {
						tags = append(tags, matches[tagIndex])
					}
					name = "prometheus" + ":" + strconv.Itoa(port)
					checkPath = "/-/healthy"
				} else if strings.Contains(path, "alertmanager") {
					name = "alertmanager"
					checkPath = "/-/healthy"
				} else {
					name = path
					checkPath = "/health"
				}
				cmdsvc := complementConsul.Service{
					Name: name,
					ID:   name + "_" + node.Name,
					IP:   *svcIP,
					Port: port,
					Tags: tags,
					Check: struct {
						Path     string
						Interval string
						Timeout  string
					}{
						Path:     checkPath,
						Interval: "60s",
						Timeout:  "10s",
					},
				}

				r.Register(&cmdsvc)
				logger.Infof("%s:%d registered.", cmdsvc.Name, cmdsvc.Port)
			}
		}
	}
	deregisterCmdsF := func() {
		if *consulAddr != "" && *registerCmds && len(d.DCmds) > 0 {
			pidAddrM, _ := daemontool.PidAddr()
			pattern := regexp.MustCompile(`prometheus_(?P<tag>[\w\.]+?)\.ya?ml`)
			for _, dcmd := range d.DCmds {

				path := dcmd.Cmd.Path
				cmdStr := dcmd.Cmd.String()
				pid := strconv.Itoa(dcmd.Cmd.Process.Pid)
				addr, ok := pidAddrM[pid]
				if !ok {
					logger.Errorf("pid not found: %s, name: %s", pid, path)
					continue
				}
				port, err := strconv.Atoi(daemontool.Parseport(addr))
				if err != nil {
					logger.Errorf("Parseport addr %s err: %v", err, addr)
					continue
				}
				var (
					name      string
					tags      []string
					checkPath string
				)

				if strings.Contains(path, "prometheus") {
					tagIndex := pattern.SubexpIndex("tag")
					matches := pattern.FindStringSubmatch(cmdStr)
					if len(matches) > 0 {
						tags = append(tags, matches[tagIndex])
					}
					name = "prometheus" + ":" + strconv.Itoa(port)
					checkPath = "/-/healthy"
				} else if strings.Contains(path, "alertmanager") {
					name = "alertmanager"
					checkPath = "/-/healthy"
				} else {
					name = path
					checkPath = "/health"
				}
				cmdsvc := complementConsul.Service{
					Name: name,
					ID:   name + "_" + node.Name,
					IP:   *svcIP,
					Port: port,
					Tags: tags,
					Check: struct {
						Path     string
						Interval string
						Timeout  string
					}{
						Path:     checkPath,
						Interval: "60s",
						Timeout:  "10s",
					},
				}

				r.Deregister(&cmdsvc)
				logger.Infof("%s:%d deregistered.", cmdsvc.Name, cmdsvc.Port)
			}
		}
	}
	registerCmdsF()
	defer deregisterCmdsF()

	chDone := make(chan struct{}, 1)
	mux.Use(cors.Default())
	go func() { // no blocking
		err := mux.Run(":" + *port)
		if err != nil {
			logger.Errorln(err)
		}
		chDone <- struct{}{}
		close(chDone)
	}()

	// 防止子进程成为僵尸进程
	defer func() {
		pid := os.Getpid()
		cancel()
		syscall.Kill(-pid, syscall.SIGTERM)
		time.Sleep(5 * time.Second)
	}()

	// 捕捉信号
	for {
		select {
		case sig := <-signCh:
			switch sig {
			// 重新加载配置文件
			case syscall.SIGHUP:
				// recover initConf panic
				var initConfPanic bool
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Errorf("Reload config failed. %v", r)
							logger.Infoln("Panic Recover. Nothing changed.")
							initConfPanic = true
						}
					}()
					initConf()
				}()
				if initConfPanic { // if initConf panic, do not reload
					break
				}

				logger.Infoln("Reloaded config.")
				cmds := config.GenerateCmds(conf)
				if len(cmds) == 0 {
					logger.Error("No cmd to run. Do not reload.")
					break
				}
				// 关闭所有子进程
				cancel()
				var wg sync.WaitGroup
				for _, dcmd := range d.DCmds {
					dcmd := dcmd // capture range variable
					if dcmd.Status == daemon.Exited {
						continue
					}
					pid := dcmd.Cmd.Process.Pid
					err := syscall.Kill(pid, syscall.SIGTERM)
					if err != nil {
						logger.Errorf("Cmd: %s Pid: %d kill failed. %v", dcmd.Cmd.String(), pid, err)
					}

					// wait for child process exited
					// if not exited, kill it after 10s
					wg.Add(1)
					go func() {
						defer wg.Done()
						// wait for child process exited
						if dcmd.Cmd == nil || dcmd.Cmd.ProcessState == nil {
							return
						}
						isExited := dcmd.Cmd.ProcessState.Exited()
						ch := make(chan struct{}, 1)
						if isExited {
							ch <- struct{}{}
						}
						select {
						case <-ch:
						case <-time.After(10 * time.Second):
							dcmd.Cmd.Process.Kill()
						}
					}()
				}
				wg.Wait()
				logger.Infoln("Ctx canceled. All child processes killed.")

				// reload Daemon and run new cmds
				ctx, cancel = context.WithCancel(context.Background())
				d.Reload(ctx, cmds)
				go d.Run()

				time.Sleep(10 * time.Second)
				registerCmdsF()
			// kill all child processes
			case syscall.SIGTERM:
				logger.Warnln("Catched a term sign, kill all child processes. ", time.Now().Format(time.DateTime))
				return // defer 会kill所有子进程
			}
		case <-chDone: // http server exited
			logger.Infoln("Web server exited.")
			return
		}
	}
}

// 守护进程上下文
func newForkCtx() *fork.Context {
	// commandName := os.Args[0]
	return &fork.Context{
		PidFileName: "daemon.pid",
		PidFilePerm: 0644,
		LogFileName: "daemon.log",
		LogFilePerm: 0644,
		WorkDir:     "./",
		Umask:       027,
		// Args:        []string{commandName},
	}
}

// 生成默认配置文件
func createConfigFile() {
	str := config.DefaultConfig
	_, err := os.Stat("daemon.yml")
	if err == nil || os.IsExist(err) {
		fmt.Println("daemon.yml existed")
		return
	}
	file, err := os.Create("daemon.yml")
	if err != nil {
		fmt.Println("create config.yml file err:", err)
		return
	}
	file.WriteString(str)
	fmt.Println("daemon.yml created.")
}

// 配置初始化
func initConf() {
	configBytes, err := os.ReadFile(*configFile)
	if err != nil {
		panic("Read config failed.")
	}
	conf, err = config.Unmarshal(configBytes)
	if err != nil {
		panic("Unmarshal config failed.")
	}
	if len(conf.Cmds) == 0 {
		panic("No cmd found.")
	}
}

// 初始化日志实例
func NewLogger(level zapcore.Level) *zap.SugaredLogger {
	writer := os.Stdout
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // 时间格式：2020-12-16T17:53:30.466+0800
	// encoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder   // 时间格式：2020-12-16T17:53:30.466+0800
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder // 在日志文件中使用大写字母记录日志级别
	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	writeSyncer := zapcore.AddSync(writer)

	core := zapcore.NewCore(encoder, writeSyncer, level)

	logger := zap.New(core, zap.AddCaller()).Sugar() // AddCaller() 显示行号和文件名
	return logger
}

func killcmd(cmd *exec.Cmd) error {
	psgrepStr := "ps -eo pid,command | grep " + "\"" + cmd.String() + "\"" + " | grep -v grep | awk '{print $1}'"
	psgrep := exec.Command("sh", "-c", psgrepStr)
	psgrepout, err := psgrep.Output()
	if err != nil {
		return err
	}
	if len(psgrepout) == 0 {
		return errors.New("psgrep no output")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(psgrepout)))
	if err != nil {
		return err
	}
	err = syscall.Kill(pid, syscall.SIGTERM)
	if err != nil {
		return err
	}
	fmt.Printf("kill %d successfully", pid)
	return nil
}
